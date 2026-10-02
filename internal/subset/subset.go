package subset

import (
	"context"
	"fmt"
	"strings"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
	"github.com/sealkit/dbseal/internal/graph"
)

// SubsettingPlan defines which rows and tables are included in the extracted subset.
type SubsettingPlan struct {
	ReferenceTables map[string]bool       // Tables that are 100% dumped (isolated dictionaries/configs)
	TablePKs        map[string][]any      // table -> list of primary key IDs to export
	VisitedPKs      map[string]map[any]bool // table -> set of visited PKs for deduplication
	InsertionOrder  []string              // Recommended topological table order
	Cycles          [][]string            // Any cyclic foreign key loops detected
	TableWarnings   map[string][]string   // table -> warnings (e.g. row_cap truncated)
}

// Engine executes relation-consistent subset extraction.
type Engine struct {
	driver drivers.Driver
	cfg    *config.Config
	graph  *graph.Graph
}

func NewEngine(driver drivers.Driver, cfg *config.Config, g *graph.Graph) *Engine {
	return &Engine{
		driver: driver,
		cfg:    cfg,
		graph:  g,
	}
}

// BuildPlan traverses the database topology starting from root targets and reference tables.
func (e *Engine) BuildPlan(ctx context.Context) (*SubsettingPlan, error) {
	plan := &SubsettingPlan{
		ReferenceTables: make(map[string]bool),
		TablePKs:        make(map[string][]any),
		VisitedPKs:      make(map[string]map[any]bool),
		TableWarnings:   make(map[string][]string),
	}

	strategy := e.cfg.Subset.Strategy
	rowCap := strategy.RowCap
	if rowCap <= 0 {
		rowCap = 10000
	}

	// 1. Process reference tables (100% full preservation regardless of graph connectivity)
	for _, refTable := range e.cfg.Subset.ReferenceTables {
		normTable := strings.ToLower(strings.TrimSpace(refTable))
		if normTable != "" {
			plan.ReferenceTables[normTable] = true
			fmt.Printf("[subset] Reference table registered (100%% export): `%s`\n", normTable)
		}
	}

	// 2. Process root targets (Seeds)
	newlyAdded := make(map[string][]any)

	for _, target := range e.cfg.Subset.Targets {
		tableName := strings.ToLower(strings.TrimSpace(target.Table))
		if tableName == "" || strings.TrimSpace(target.Query) == "" {
			continue
		}

		fmt.Printf("[subset] Querying root target `%s` with: %s\n", tableName, target.Query)
		ids, err := e.driver.QueryIDs(ctx, target.Query)
		if err != nil {
			return nil, fmt.Errorf("failed root query for table %q: %w", tableName, err)
		}

		if plan.VisitedPKs[tableName] == nil {
			plan.VisitedPKs[tableName] = make(map[any]bool)
		}

		for _, id := range ids {
			if !plan.VisitedPKs[tableName][id] {
				plan.VisitedPKs[tableName][id] = true
				plan.TablePKs[tableName] = append(plan.TablePKs[tableName], id)
				newlyAdded[tableName] = append(newlyAdded[tableName], id)
			}
		}

		fmt.Printf("   -> Found %d initial seed rows for `%s`\n", len(ids), tableName)
	}

	// 3. Bidirectional graph traversal (Upstream N:1 completion + Downstream 1:N cascade)
	depth := 0
	maxDepth := strategy.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 4
	}

	for len(newlyAdded) > 0 && depth < maxDepth {
		depth++
		nextNewlyAdded := make(map[string][]any)

		for currentTable, currentIDs := range newlyAdded {
			if len(currentIDs) == 0 {
				continue
			}

			// --- A. Downstream 1:N Cascade (e.g. users -> orders, posts -> comments) ---
			if strategy.FollowDownstream {
				children := e.graph.GetChildren(currentTable)
				for _, rel := range children {
					childTable := strings.ToLower(rel.FromTable)
					if e.isExcluded(childTable) || plan.ReferenceTables[childTable] {
						continue
					}

					if plan.VisitedPKs[childTable] == nil {
						plan.VisitedPKs[childTable] = make(map[any]bool)
					}

					// Check row_cap before expanding
					if len(plan.TablePKs[childTable]) >= rowCap {
						warnMsg := fmt.Sprintf("Table `%s` reached row_cap limit (%d rows), skipping further downstream expansion", childTable, rowCap)
						plan.TableWarnings[childTable] = append(plan.TableWarnings[childTable], warnMsg)
						if strings.EqualFold(strategy.RowCapExceeded, "fail") {
							return nil, fmt.Errorf("subsetting aborted: %s", warnMsg)
						}
						continue
					}

					// Query child rows where foreign_key IN (currentIDs)
					childPKCol, err := e.getPrimaryKeyColumn(ctx, childTable)
					if err != nil {
						continue
					}

					query := fmt.Sprintf("SELECT `%s` FROM `%s` WHERE `%s` IN (%s)",
						childPKCol, childTable, rel.FromColumn, formatIDList(currentIDs))
					childIDs, err := e.driver.QueryIDs(ctx, query)
					if err != nil {
						fmt.Printf("  ⚠️ Downstream query failed for `%s`.`%s`: %v\n", childTable, rel.FromColumn, err)
						continue
					}

					var addedForChild []any
					truncated := false
					for _, cid := range childIDs {
						if !plan.VisitedPKs[childTable][cid] {
							if len(plan.TablePKs[childTable]) >= rowCap {
								truncated = true
								break
							}
							plan.VisitedPKs[childTable][cid] = true
							plan.TablePKs[childTable] = append(plan.TablePKs[childTable], cid)
							addedForChild = append(addedForChild, cid)
						}
					}

					if truncated {
						warnMsg := fmt.Sprintf("Table `%s` hit row_cap limit (%d rows), truncated downstream expansion", childTable, rowCap)
						plan.TableWarnings[childTable] = append(plan.TableWarnings[childTable], warnMsg)
						if strings.EqualFold(strategy.RowCapExceeded, "fail") {
							return nil, fmt.Errorf("subsetting aborted: %s", warnMsg)
						}
					}

					if len(addedForChild) > 0 {
						nextNewlyAdded[childTable] = append(nextNewlyAdded[childTable], addedForChild...)
						fmt.Printf("   [depth %d] Cascaded 1:N -> `%s` (+%d rows via %s.%s)\n",
							depth, childTable, len(addedForChild), childTable, rel.FromColumn)
					}
				}
			}

			// --- B. Upstream N:1 Dictionary Completion (e.g. orders -> users, products -> categories) ---
			if strategy.CompleteDictionaries {
				parents := e.graph.GetParents(currentTable)
				for _, rel := range parents {
					parentTable := strings.ToLower(rel.ToTable)
					if e.isExcluded(parentTable) || plan.ReferenceTables[parentTable] {
						continue
					}

					if plan.VisitedPKs[parentTable] == nil {
						plan.VisitedPKs[parentTable] = make(map[any]bool)
					}

					// Query distinct foreign key values from currentTable to find parent IDs
					parentPKCol, err := e.getPrimaryKeyColumn(ctx, parentTable)
					if err != nil {
						continue
					}

					query := fmt.Sprintf("SELECT DISTINCT `%s` FROM `%s` WHERE `%s` IN (%s) AND `%s` IS NOT NULL",
						rel.FromColumn, currentTable, e.getPrimaryKeyColumnName(ctx, currentTable), formatIDList(currentIDs), rel.FromColumn)
					parentRefIDs, err := e.driver.QueryIDs(ctx, query)
					if err != nil {
						continue
					}

					var addedForParent []any
					for _, pid := range parentRefIDs {
						if !plan.VisitedPKs[parentTable][pid] {
							plan.VisitedPKs[parentTable][pid] = true
							plan.TablePKs[parentTable] = append(plan.TablePKs[parentTable], pid)
							addedForParent = append(addedForParent, pid)
						}
					}

					if len(addedForParent) > 0 {
						nextNewlyAdded[parentTable] = append(nextNewlyAdded[parentTable], addedForParent...)
						fmt.Printf("   [depth %d] Completed N:1 -> `%s` (+%d parent rows via %s.%s -> %s.%s)\n",
							depth, parentTable, len(addedForParent), currentTable, rel.FromColumn, parentTable, parentPKCol)
					}
				}
			}
		}

		newlyAdded = nextNewlyAdded
	}

	// 4. Compute topological table order and detect cycles
	plan.InsertionOrder = e.graph.TopologicalSort()
	plan.Cycles = e.graph.DetectCycles()

	if len(plan.Cycles) > 0 {
		fmt.Printf("[subset] Detected %d foreign key cycle(s):\n", len(plan.Cycles))
		for idx, cycle := range plan.Cycles {
			fmt.Printf("   Cycle #%d: %s\n", idx+1, strings.Join(cycle, " <-> "))
		}
	}

	return plan, nil
}

func (e *Engine) isExcluded(table string) bool {
	for _, pattern := range e.cfg.Subset.Strategy.ExcludeTables {
		if strings.EqualFold(pattern, table) {
			return true
		}
	}
	return false
}

func (e *Engine) getPrimaryKeyColumn(ctx context.Context, table string) (string, error) {
	schema, err := e.driver.GetTableSchema(ctx, table)
	if err != nil {
		return "", err
	}
	if len(schema.PKColumns) > 0 {
		return schema.PKColumns[0], nil
	}
	return "id", nil
}

func (e *Engine) getPrimaryKeyColumnName(ctx context.Context, table string) string {
	pk, err := e.getPrimaryKeyColumn(ctx, table)
	if err != nil || pk == "" {
		return "id"
	}
	return pk
}

func formatIDList(ids []any) string {
	var parts []string
	for _, id := range ids {
		switch v := id.(type) {
		case string:
			parts = append(parts, fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")))
		default:
			parts = append(parts, fmt.Sprintf("%v", v))
		}
	}
	if len(parts) == 0 {
		return "NULL"
	}
	return strings.Join(parts, ", ")
}
