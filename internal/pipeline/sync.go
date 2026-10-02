package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/correctness"
	"github.com/sealkit/dbseal/internal/drift"
	"github.com/sealkit/dbseal/internal/drivers"
	"github.com/sealkit/dbseal/internal/masking"
	"github.com/sealkit/dbseal/pkg/extension"
)

// SyncOptions contains options for dbseal sync command.
type SyncOptions struct {
	ConfigPath string
	Tables     []string
}

// ExecuteSync executes in-memory direct database refresh from Source to Target with drift adaptation.
func ExecuteSync(ctx context.Context, cfg *config.Config, opts SyncOptions) error {
	start := time.Now()

	if strings.TrimSpace(cfg.Target.DSN) == "" {
		return fmt.Errorf("target database DSN is required for 'dbseal sync' (configure target.dsn in YAML or set TARGET_DB_URL)")
	}

	// 1. Initialize Source Driver
	sourceDriverName := cfg.Source.Driver
	if sourceDriverName == "" {
		sourceDriverName = "mysql"
	}
	sourceDriver, err := drivers.Get(sourceDriverName)
	if err != nil {
		return err
	}

	fmt.Printf("[dbseal sync] Connecting to Source database (%s)...\n", sourceDriverName)
	if err := sourceDriver.Open(ctx, cfg.Source.DSN, cfg.Source.Options); err != nil {
		return fmt.Errorf("source connection failed: %w", err)
	}
	defer sourceDriver.Close()

	// 2. Initialize Target Driver
	targetDriverName := cfg.Target.Driver
	if targetDriverName == "" {
		targetDriverName = sourceDriverName
	}
	targetDriver, err := drivers.Get(targetDriverName)
	if err != nil {
		return err
	}

	fmt.Printf("[dbseal sync] Connecting to Target test database (%s)...\n", targetDriverName)
	targetOpts := config.SourceOptions{MaxOpenConns: 5}
	if err := targetDriver.Open(ctx, cfg.Target.DSN, targetOpts); err != nil {
		return fmt.Errorf("target connection failed: %w", err)
	}
	defer targetDriver.Close()

	// 3. Disable foreign key checks on Target for clean wipe and insert
	if err := targetDriver.SetForeignKeyChecks(ctx, false); err != nil {
		fmt.Printf("  ⚠️ Could not disable target foreign key checks: %v\n", err)
	}
	defer targetDriver.SetForeignKeyChecks(ctx, true)

	// 4. Initialize Engines
	maskEngine := masking.NewEngine(cfg.Masking.Tables)
	driftAdapter := drift.NewAdapter(sourceDriver, targetDriver)
	guard := correctness.NewGuard()

	// 5. Determine Tables to Sync
	allTables, err := sourceDriver.GetTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tables from source: %w", err)
	}
	targetTables := filterTables(allTables, opts.Tables)

	fmt.Printf("[dbseal sync] Direct in-memory sync pipeline initialized (%d tables)\n\n", len(targetTables))

	var totalSyncedRows int64

	syncTable := func(tableCtx context.Context, table string) (int64, time.Duration, []string, error) {
		tStart := time.Now()
		normTable := strings.ToLower(table)

		// A. Drift Analysis
		driftRes, err := driftAdapter.AnalyzeDrift(tableCtx, normTable)
		if err != nil {
			return 0, time.Since(tStart), nil, err
		}

		if len(driftRes.CommonCols) == 0 {
			return 0, time.Since(tStart), nil, nil
		}

		// B. Target Wipe Phase (Safe DELETE FROM, no TRUNCATE)
		if err := targetDriver.WipeTable(tableCtx, normTable); err != nil {
			return 0, time.Since(tStart), nil, err
		}

		// C. In-Memory Streaming & Direct Write Phase
		sourceSchema, err := sourceDriver.GetTableSchema(tableCtx, normTable)
		if err != nil {
			return 0, time.Since(tStart), nil, err
		}
		sourceCols := sourceSchema.GetInsertableColumns()

		pkCol := "id"
		hasAutoIncr := false
		for _, col := range sourceSchema.Columns {
			if col.IsAutoIncr {
				hasAutoIncr = true
				pkCol = col.Name
				break
			}
		}

		var tableRows int64
		var maskedCols []string
		if colRules, ok := cfg.Masking.Tables[normTable]; ok {
			for col := range colRules {
				maskedCols = append(maskedCols, col)
			}
		}

		err = sourceDriver.StreamRows(tableCtx, normTable, sourceCols, "", 2000, func(batch [][]any) error {
			projectedBatch := make([][]any, 0, len(batch))

			for _, row := range batch {
				for colIdx, colName := range sourceCols {
					maskedVal, err := maskEngine.MaskColumn(normTable, colName, row[colIdx])
					if err != nil {
						return err
					}
					row[colIdx] = maskedVal

					if hasAutoIncr && strings.EqualFold(colName, pkCol) {
						guard.TrackID(normTable, maskedVal)
					}
				}

				projectedRow := drift.ProjectRow(row, sourceCols, driftRes.CommonCols)
				projectedBatch = append(projectedBatch, projectedRow)
			}

			insertSQL, err := targetDriver.FormatInsertSQL(normTable, driftRes.CommonCols, projectedBatch)
			if err != nil {
				return err
			}

			if err := targetDriver.ExecuteSQL(tableCtx, insertSQL); err != nil {
				return fmt.Errorf("target insert failed on `%s`: %w", normTable, err)
			}

			tableRows += int64(len(batch))
			return nil
		})

		if err != nil {
			return tableRows, time.Since(tStart), maskedCols, err
		}

		return tableRows, time.Since(tStart), maskedCols, nil
	}

	resetSeeds := func(resetCtx context.Context) error {
		for _, table := range targetTables {
			normTable := strings.ToLower(table)
			if maxID, ok := guard.GetMaxID(normTable); ok && maxID > 0 {
				_ = targetDriver.ResetAutoIncrement(resetCtx, normTable, maxID+1)
			}
		}
		return nil
	}

	// 6. If Pro parallel pipeline extension is registered, delegate execution to it!
	if p := extension.GetPipeline(); p != nil {
		plan := extension.SyncPlan{
			SourceName: sourceDriverName,
			TargetName: targetDriverName,
			Tables:     targetTables,
			SyncTable:  syncTable,
			ResetSeeds: resetSeeds,
		}
		return p.Run(ctx, plan)
	}

	// 7. OSS Sequential Execution
	for i, table := range targetTables {
		normTable := strings.ToLower(table)
		fmt.Printf("[%d/%d] Synchronizing table `%s`:\n", i+1, len(targetTables), normTable)
		fmt.Printf("   • [Stream] Pulling, masking, and direct writing... ")

		rows, _, _, err := syncTable(ctx, normTable)
		if err != nil {
			fmt.Printf("FAILED ❌ (%v)\n", err)
			return err
		}
		totalSyncedRows += rows
		fmt.Printf("DONE ✅ (%d rows written)\n", rows)
	}

	_ = resetSeeds(ctx)

	duration := time.Since(start).Round(time.Millisecond)

	fmt.Printf("\n✨ In-Memory Sync Completed in %s!\n", duration)
	fmt.Printf("   - Tables Refreshed: %d\n", len(targetTables))
	fmt.Printf("   - Total Rows Synced: %d rows\n", totalSyncedRows)
	fmt.Printf("   - Intermediate Disk IO: 0 MB (100%% In-Memory Streaming)\n\n")

	return nil
}
