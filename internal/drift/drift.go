package drift

import (
	"context"
	"fmt"
	"strings"

	"github.com/sealkit/dbseal/internal/drivers"
)

// TableDriftResult contains schema drift analysis for a single table between Source and Target.
type TableDriftResult struct {
	TableName                 string
	CommonCols                []string // Columns existing in both that are insertable (Intersection)
	SourceOnlyCols            []string // Deprecated columns present in Source but removed in Target (Auto-Skipped)
	TargetOnlyCols            []string // New columns present in Target (Staging Migration)
	MissingNotNullNoDefault   []string // Target columns that are NOT NULL without DEFAULT (Require fallback)
}

// Adapter handles schema drift adaptation and column projection.
type Adapter struct {
	sourceDriver drivers.Driver
	targetDriver drivers.Driver
}

func NewAdapter(source, target drivers.Driver) *Adapter {
	return &Adapter{
		sourceDriver: source,
		targetDriver: target,
	}
}

// AnalyzeDrift compares the schema of a table between Source and Target database.
func (a *Adapter) AnalyzeDrift(ctx context.Context, table string) (*TableDriftResult, error) {
	normTable := strings.ToLower(table)

	sourceSchema, err := a.sourceDriver.GetTableSchema(ctx, normTable)
	if err != nil {
		return nil, fmt.Errorf("failed to get source schema for `%s`: %w", normTable, err)
	}

	targetSchema, err := a.targetDriver.GetTableSchema(ctx, normTable)
	if err != nil {
		return nil, fmt.Errorf("failed to get target schema for `%s`: %w", normTable, err)
	}

	sourceInsertable := sourceSchema.GetInsertableColumns()
	targetInsertable := targetSchema.GetInsertableColumns()

	targetLookup := make(map[string]drivers.ColumnInfo)
	for _, col := range targetSchema.Columns {
		targetLookup[strings.ToLower(col.Name)] = col
	}

	sourceLookup := make(map[string]bool)
	for _, c := range sourceInsertable {
		sourceLookup[strings.ToLower(c)] = true
	}

	result := &TableDriftResult{
		TableName: normTable,
	}

	// 1. Calculate Common Columns (Intersection) and Source-Only Columns
	for _, colName := range sourceInsertable {
		lower := strings.ToLower(colName)
		if targetCol, existsInTarget := targetLookup[lower]; existsInTarget && !targetCol.IsGenerated {
			result.CommonCols = append(result.CommonCols, targetCol.Name)
		} else {
			result.SourceOnlyCols = append(result.SourceOnlyCols, colName)
		}
	}

	// 2. Identify Target-Only Columns (new migration fields in Staging)
	for _, colName := range targetInsertable {
		lower := strings.ToLower(colName)
		if !sourceLookup[lower] {
			result.TargetOnlyCols = append(result.TargetOnlyCols, colName)

			colInfo := targetLookup[lower]
			if !colInfo.IsNullable && colInfo.DefaultValue == nil && !colInfo.IsAutoIncr && !colInfo.IsGenerated {
				result.MissingNotNullNoDefault = append(result.MissingNotNullNoDefault, colName)
			}
		}
	}

	return result, nil
}

// ProjectRow extracts values from sourceRow corresponding to commonCols.
func ProjectRow(sourceRow []any, sourceCols []string, commonCols []string) []any {
	if len(sourceCols) == len(commonCols) {
		// Optimization: if identical, return as is
		return sourceRow
	}

	sourceIndex := make(map[string]int, len(sourceCols))
	for i, col := range sourceCols {
		sourceIndex[strings.ToLower(col)] = i
	}

	projected := make([]any, len(commonCols))
	for i, col := range commonCols {
		idx, ok := sourceIndex[strings.ToLower(col)]
		if ok && idx < len(sourceRow) {
			projected[i] = sourceRow[idx]
		} else {
			projected[i] = nil
		}
	}

	return projected
}
