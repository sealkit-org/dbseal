package pipeline

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/correctness"
	"github.com/sealkit/dbseal/internal/drivers"
	"github.com/sealkit/dbseal/internal/graph"
	"github.com/sealkit/dbseal/internal/masking"
	"github.com/sealkit/dbseal/internal/sampling"
	"github.com/sealkit/dbseal/internal/subset"
)

// SliceOptions contains options for slice command.
type SliceOptions struct {
	ConfigPath string
	OutputPath string
}

// ExecuteSlice executes the full subset extraction and writes self-healing standalone SQL.
func ExecuteSlice(ctx context.Context, cfg *config.Config, opts SliceOptions) error {
	start := time.Now()

	// 1. Resolve output file
	outFile := opts.OutputPath
	if outFile == "" {
		outFile = cfg.Output.File
	}
	if outFile == "" {
		outFile = "./dev_seed_data.sql.gz"
	}

	if err := os.MkdirAll(filepath.Dir(outFile), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %w", outFile, err)
	}
	defer f.Close()

	var writer io.Writer = f
	var gzWriter *gzip.Writer
	isGzip := strings.HasSuffix(strings.ToLower(outFile), ".gz")
	if isGzip {
		gzWriter = gzip.NewWriter(f)
		writer = gzWriter
		defer gzWriter.Close()
	}

	// 2. Initialize Driver
	driverName := cfg.Source.Driver
	if driverName == "" {
		driverName = "mysql"
	}

	driver, err := drivers.Get(driverName)
	if err != nil {
		return err
	}

	fmt.Printf("[dbseal slice] Connecting to %s database...\n", driverName)
	if err := driver.Open(ctx, cfg.Source.DSN, cfg.Source.Options); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer driver.Close()

	// 3. Scan Physical & Virtual Foreign Keys
	fmt.Println("[dbseal slice] Scanning physical foreign keys...")
	fks, err := driver.GetForeignKeys(ctx)
	if err != nil {
		return fmt.Errorf("failed to scan foreign keys: %w", err)
	}
	fmt.Printf("   -> Discovered %d physical foreign key constraint(s)\n", len(fks))

	g := graph.NewGraph()
	for _, fk := range fks {
		g.AddRelationship(graph.Relationship{
			FromTable:  fk.FromTable,
			FromColumn: fk.FromColumn,
			ToTable:    fk.ToTable,
			ToColumn:   fk.ToColumn,
			IsVirtual:  false,
		})
	}

	for _, vfk := range cfg.Relations.Virtual {
		partsFrom := splitTableCol(vfk.From)
		partsTo := splitTableCol(vfk.To)
		if len(partsFrom) == 2 && len(partsTo) == 2 {
			g.AddRelationship(graph.Relationship{
				FromTable:  partsFrom[0],
				FromColumn: partsFrom[1],
				ToTable:    partsTo[0],
				ToColumn:   partsTo[1],
				IsVirtual:  true,
				Condition:  vfk.Condition,
			})
			fmt.Printf("   -> Registered virtual foreign key: %s.%s -> %s.%s\n",
				partsFrom[0], partsFrom[1], partsTo[0], partsTo[1])
		}
	}

	// 4. Heuristic Auto-Detection & Sampling Validation
	if cfg.Relations.AutoDetect.Enabled {
		fmt.Println("[dbseal slice] Running heuristic virtual foreign key inference & sampling validation...")
		allTables, err := driver.GetTables(ctx)
		if err == nil {
			validator := sampling.NewValidator(driver, cfg)
			validated, err := validator.AutoDetectAndValidate(ctx, allTables)
			if err == nil {
				for _, v := range validated {
					if v.Status == "confirmed" {
						fmt.Printf("   ✅ [confirmed] %s.%s -> %s.%s (match rate: %.1f%%, %d/%d sampled)\n",
							v.FromTable, v.FromColumn, v.ToTable, v.ToColumn, v.MatchRate*100, v.MatchedCount, v.SampledCount)
						g.AddRelationship(graph.Relationship{
							FromTable:  v.FromTable,
							FromColumn: v.FromColumn,
							ToTable:    v.ToTable,
							ToColumn:   v.ToColumn,
							IsVirtual:  true,
						})
					} else {
						fmt.Printf("   ⚠️ [%s] %s.%s -> %s.%s (match rate: %.1f%%) - %s\n",
							v.Status, v.FromTable, v.FromColumn, v.ToTable, v.ToColumn, v.MatchRate*100, v.Reason)
					}
				}
			}
		}
	}

	// 5. Build Subsetting Plan
	fmt.Println("\n[dbseal slice] Running topological graph traversal...")
	engine := subset.NewEngine(driver, cfg, g)
	plan, err := engine.BuildPlan(ctx)
	if err != nil {
		return fmt.Errorf("failed to build subsetting plan: %w", err)
	}

	// 5. Initialize Masking Engine & Correctness Guard
	maskEngine := masking.NewEngine(cfg.Masking.Tables)
	guard := correctness.NewGuard()

	// 6. Write Session Header
	if _, err := io.WriteString(writer, driver.SessionHeaderSQL()); err != nil {
		return err
	}

	// 7. Stream tables according to topological order
	fmt.Printf("\n[dbseal slice] Streaming subset data into %s...\n", outFile)

	var totalExportedRows int64
	var autoIncrTables []string

	// Track all tables to process: topological order first, then any reference tables not in order
	processedTables := make(map[string]bool)
	var orderedTables []string

	for _, t := range plan.InsertionOrder {
		orderedTables = append(orderedTables, t)
		processedTables[strings.ToLower(t)] = true
	}
	for refTable := range plan.ReferenceTables {
		if !processedTables[refTable] {
			orderedTables = append(orderedTables, refTable)
			processedTables[refTable] = true
		}
	}

	for _, table := range orderedTables {
		normTable := strings.ToLower(table)
		isRef := plan.ReferenceTables[normTable]
		targetIDs := plan.TablePKs[normTable]

		if !isRef && len(targetIDs) == 0 {
			continue // No rows sampled for this table
		}

		schema, err := driver.GetTableSchema(ctx, normTable)
		if err != nil {
			fmt.Printf("  ⚠️ Skipping table `%s` (schema error: %v)\n", normTable, err)
			continue
		}

		insertableCols := schema.GetInsertableColumns()
		if len(insertableCols) == 0 {
			continue
		}

		// Check if table has auto-increment primary key
		pkCol := "id"
		hasAutoIncr := false
		for _, col := range schema.Columns {
			if col.IsAutoIncr {
				hasAutoIncr = true
				pkCol = col.Name
				break
			}
			if col.IsPrimaryKey && pkCol == "id" {
				pkCol = col.Name
			}
		}
		if hasAutoIncr {
			autoIncrTables = append(autoIncrTables, normTable)
		}

		// Write Table Section Header
		tag := "subset"
		if isRef {
			tag = "reference 100%"
		}
		fmt.Printf("  • Writing `%s` (%s)... ", normTable, tag)

		tableBanner := fmt.Sprintf("\n-- --------------------------------------------------------\n-- Table `%s` (%s)\n-- --------------------------------------------------------\n", normTable, tag)
		if _, err := io.WriteString(writer, tableBanner); err != nil {
			return err
		}

		var tableRowsCount int64

		if isRef {
			// Case A: Stream 100% of reference table
			err = driver.StreamRows(ctx, normTable, insertableCols, "", 2000, func(batch [][]any) error {
				for _, row := range batch {
					for colIdx, colName := range insertableCols {
						maskedVal, err := maskEngine.MaskColumn(normTable, colName, row[colIdx])
						if err != nil {
							return err
						}
						row[colIdx] = maskedVal

						if hasAutoIncr && strings.EqualFold(colName, pkCol) {
							guard.TrackID(normTable, maskedVal)
						}
					}
				}

				insertSQL, err := driver.FormatInsertSQL(normTable, insertableCols, batch)
				if err != nil {
					return err
				}
				if _, err := io.WriteString(writer, insertSQL); err != nil {
					return err
				}

				tableRowsCount += int64(len(batch))
				return nil
			})
		} else {
			// Case B: Chunked streaming for subsetted IDs (1000 IDs per chunk to keep query sizes safe)
			chunkSize := 1000
			for i := 0; i < len(targetIDs); i += chunkSize {
				end := i + chunkSize
				if end > len(targetIDs) {
					end = len(targetIDs)
				}
				chunk := targetIDs[i:end]

				whereClause := fmt.Sprintf("`%s` IN (%s)", pkCol, formatIDListForSQL(chunk))
				err = driver.StreamRows(ctx, normTable, insertableCols, whereClause, 2000, func(batch [][]any) error {
					for _, row := range batch {
						for colIdx, colName := range insertableCols {
							maskedVal, err := maskEngine.MaskColumn(normTable, colName, row[colIdx])
							if err != nil {
								return err
							}
							row[colIdx] = maskedVal

							if hasAutoIncr && strings.EqualFold(colName, pkCol) {
								guard.TrackID(normTable, maskedVal)
							}
						}
					}

					insertSQL, err := driver.FormatInsertSQL(normTable, insertableCols, batch)
					if err != nil {
						return err
					}
					if _, err := io.WriteString(writer, insertSQL); err != nil {
						return err
					}

					tableRowsCount += int64(len(batch))
					return nil
				})
				if err != nil {
					break
				}
			}
		}

		if err != nil {
			fmt.Printf("FAILED ❌ (%v)\n", err)
			return err
		}

		totalExportedRows += tableRowsCount
		fmt.Printf("DONE ✅ (%d rows)\n", tableRowsCount)
	}

	// 8. Append Self-Healing SQL Footer (Auto-increment seeds reset & cycle resolution)
	selfHealingEnabled := cfg.Output.SelfHealing
	if selfHealingEnabled || cfg.Output.SelfHealing == false && cfg.Output.Format == "sql.gz" {
		selfHealingEnabled = true // default true for self-healing standalone files
	}

	if selfHealingEnabled {
		selfHealingSQL := guard.GenerateSelfHealingSQL(autoIncrTables)
		if _, err := io.WriteString(writer, selfHealingSQL); err != nil {
			return err
		}
		fmt.Printf("\n[dbseal slice] Appended self-healing SQL: %d AUTO_INCREMENT seed(s) calculated\n", len(autoIncrTables))
	}

	// 9. Write Session Footer
	if _, err := io.WriteString(writer, driver.SessionFooterSQL()); err != nil {
		return err
	}

	if gzWriter != nil {
		if err := gzWriter.Flush(); err != nil {
			return err
		}
	}

	// 10. Summary Report
	fi, _ := f.Stat()
	fileSizeMB := float64(fi.Size()) / (1024 * 1024)
	duration := time.Since(start).Round(time.Millisecond)

	fmt.Printf("\n🎉 Slice Subsetting Completed in %s!\n", duration)
	fmt.Printf("   - Subsetting Rows:  %d rows extracted\n", totalExportedRows)
	fmt.Printf("   - Output File:      %s (%.2f MB)\n", outFile, fileSizeMB)
	fmt.Printf("   - Ready to import:  mysql -u root < %s (Zero tools required!)\n\n", outFile)

	return nil
}

func formatIDListForSQL(ids []any) string {
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

func splitTableCol(s string) []string {
	idx := strings.Index(s, ".")
	if idx > 0 && idx < len(s)-1 {
		return []string{s[:idx], s[idx+1:]}
	}
	return []string{s}
}
