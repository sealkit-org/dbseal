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
	"github.com/sealkit/dbseal/internal/drivers"
	"github.com/sealkit/dbseal/internal/masking"
)

// DumpOptions configures the full database dump execution.
type DumpOptions struct {
	ConfigPath string
	OutputPath string
	Tables     []string
}

// ExecuteDump performs a full database anonymized dump into a compressed .sql.gz or .sql file.
func ExecuteDump(ctx context.Context, cfg *config.Config, opts DumpOptions) error {
	start := time.Now()

	// 1. Resolve output file
	outFile := opts.OutputPath
	if outFile == "" {
		outFile = cfg.Output.File
	}
	if outFile == "" {
		outFile = "./dbseal_dump.sql.gz"
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

	fmt.Printf("[dbseal dump] Connecting to %s database...\n", driverName)
	if err := driver.Open(ctx, cfg.Source.DSN, cfg.Source.Options); err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer driver.Close()

	// 3. Initialize Masking Engine
	maskEngine := masking.NewEngine(cfg.Masking.Tables)

	// 4. Determine list of tables to dump
	allTables, err := driver.GetTables(ctx)
	if err != nil {
		return fmt.Errorf("failed to list database tables: %w", err)
	}

	targetTables := filterTables(allTables, opts.Tables)
	fmt.Printf("[dbseal dump] Found %d tables to export into %s\n\n", len(targetTables), outFile)

	// 5. Write Session Header
	if _, err := io.WriteString(writer, driver.SessionHeaderSQL()); err != nil {
		return err
	}

	// 6. Stream and Dump Tables
	var totalRows int64

	for i, table := range targetTables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		schema, err := driver.GetTableSchema(ctx, table)
		if err != nil {
			fmt.Printf("  ⚠️ Skipping table %q (schema error: %v)\n", table, err)
			continue
		}

		insertableCols := schema.GetInsertableColumns()
		if len(insertableCols) == 0 {
			continue
		}

		fmt.Printf("  [%d/%d] Dumping table `%s`... ", i+1, len(targetTables), table)

		tableHeader := fmt.Sprintf("\n-- --------------------------------------------------------\n-- Table structure and data for `%s`\n-- --------------------------------------------------------\n", table)
		if _, err := io.WriteString(writer, tableHeader); err != nil {
			return err
		}

		var tableRows int64

		err = driver.StreamRows(ctx, table, insertableCols, "", 2000, func(batch [][]any) error {
			// Apply column masking in-place for each row in batch
			for _, row := range batch {
				for colIdx, colName := range insertableCols {
					originalVal := row[colIdx]
					maskedVal, err := maskEngine.MaskColumn(table, colName, originalVal)
					if err != nil {
						return fmt.Errorf("masking error on `%s`.`%s`: %w", table, colName, err)
					}
					row[colIdx] = maskedVal
				}
			}

			// Generate batch INSERT SQL
			insertSQL, err := driver.FormatInsertSQL(table, insertableCols, batch)
			if err != nil {
				return fmt.Errorf("failed to format insert SQL: %w", err)
			}

			if _, err := io.WriteString(writer, insertSQL); err != nil {
				return fmt.Errorf("failed to write SQL output: %w", err)
			}

			tableRows += int64(len(batch))
			return nil
		})

		if err != nil {
			fmt.Printf("FAILED ❌ (%v)\n", err)
			return err
		}

		totalRows += tableRows
		fmt.Printf("DONE ✅ (%d rows)\n", tableRows)
	}

	// 7. Write Session Footer
	if _, err := io.WriteString(writer, driver.SessionFooterSQL()); err != nil {
		return err
	}

	if gzWriter != nil {
		if err := gzWriter.Flush(); err != nil {
			return err
		}
	}

	// 8. Stats summary
	fi, _ := f.Stat()
	fileSizeMB := float64(fi.Size()) / (1024 * 1024)
	duration := time.Since(start).Round(time.Millisecond)

	fmt.Printf("\n✨ Dump Completed in %s!\n", duration)
	fmt.Printf("   - Total Tables: %d\n", len(targetTables))
	fmt.Printf("   - Total Rows:   %d\n", totalRows)
	fmt.Printf("   - Output File:  %s (%.2f MB)\n\n", outFile, fileSizeMB)

	return nil
}

func filterTables(all []string, explicit []string) []string {
	if len(explicit) == 0 {
		return all
	}
	lookup := make(map[string]bool)
	for _, t := range explicit {
		lookup[strings.ToLower(t)] = true
	}
	var res []string
	for _, t := range all {
		if lookup[strings.ToLower(t)] {
			res = append(res, t)
		}
	}
	return res
}
