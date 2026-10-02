package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sealkit/dbseal/internal/config"
)

func printValidateUsage() {
	fmt.Println("dbseal validate 🦭 - Verify syntax, rules, and database connectivity")
	fmt.Println("\nUsage:")
	fmt.Println("  dbseal validate [config-file] [flags]")
	fmt.Println("\nArguments:")
	fmt.Println("  config-file       Path to configuration file (default: dbseal.yaml)")
	fmt.Println("\nFlags:")
	fmt.Println("  -c, --config      Path to configuration file (alternative to positional argument)")
	fmt.Println("  -m, --mode        Execution mode to validate: 'slice', 'sync', or 'all' (default: all)")
	fmt.Println("  -p, --ping        Test actual network connectivity to source/target databases")
	fmt.Println("  -h, --help        Show help for validate command")
	fmt.Println("\nExamples:")
	fmt.Println("  dbseal validate")
	fmt.Println("  dbseal validate staging.yaml")
	fmt.Println("  dbseal validate --config staging.yaml --mode sync")
	fmt.Println("  dbseal validate --mode slice --ping")
}

func runValidate(ctx context.Context, args []string) error {
	ping := false
	mode := ""
	target := ""

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			printValidateUsage()
			return nil
		case arg == "-p" || arg == "--ping" || arg == "--connect":
			ping = true
		case arg == "-c" || arg == "--config":
			if i+1 < len(args) {
				target = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--config="):
			target = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-c="):
			target = strings.TrimPrefix(arg, "-c=")
		case arg == "-m" || arg == "--mode":
			if i+1 < len(args) {
				mode = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--mode="):
			mode = strings.TrimPrefix(arg, "--mode=")
		case strings.HasPrefix(arg, "-m="):
			mode = strings.TrimPrefix(arg, "-m=")
		case !strings.HasPrefix(arg, "-") && target == "":
			target = arg
		}
	}

	if target == "" {
		target = "dbseal.yaml"
	}

	if !strings.HasSuffix(target, ".yaml") && !strings.HasSuffix(target, ".yml") {
		target = target + ".yaml"
	}

	fmt.Printf("[dbseal validate] Validating %q...\n", target)

	cfg, err := config.LoadConfig(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Configuration Load Failed:\n  %v\n\n", err)
		return fmt.Errorf("configuration syntax error in %s", target)
	}

	report := cfg.Validate(mode)

	// Display Issues
	if len(report.Issues) > 0 {
		fmt.Println("\nInspection Results:")
		for _, issue := range report.Issues {
			if issue.Level == "ERROR" {
				fmt.Printf("  ❌ [ERROR] [%s] %s\n", issue.Section, issue.Message)
			} else {
				fmt.Printf("  ⚠️  [WARN]  [%s] %s\n", issue.Section, issue.Message)
			}
		}
		fmt.Println()
	}

	if report.HasError {
		errorCount := 0
		for _, i := range report.Issues {
			if i.Level == "ERROR" {
				errorCount++
			}
		}
		return fmt.Errorf("configuration validation failed with %d error(s)", errorCount)
	}

	// Execution Mode Status
	fmt.Println("\nExecution Mode Readiness:")
	if report.SliceReady {
		fmt.Printf("  ✓ [slice] %s\n", report.SliceReason)
	} else {
		fmt.Printf("  ❌ [slice] %s\n", report.SliceReason)
	}

	if report.SyncReady {
		fmt.Printf("  ✓ [sync]  %s\n", report.SyncReason)
	} else if strings.TrimSpace(cfg.Target.DSN) == "" {
		fmt.Printf("  ℹ️  [sync]  %s\n", report.SyncReason)
	} else {
		fmt.Printf("  ❌ [sync]  %s\n", report.SyncReason)
	}

	// Ping database if requested
	if ping {
		fmt.Println("\nDatabase Connectivity Check:")
		if err := testPing(ctx, cfg.Source.Driver, cfg.Source.DSN); err != nil {
			fmt.Printf("  ❌ Source DB (%s): Connection Failed -> %v\n", cfg.Source.Driver, err)
			return fmt.Errorf("source database connectivity test failed")
		}
		fmt.Printf("  ✓ Source DB (%s): Connection Successful\n", cfg.Source.Driver)

		if cfg.Target.DSN != "" {
			if err := testPing(ctx, cfg.Target.Driver, cfg.Target.DSN); err != nil {
				fmt.Printf("  ❌ Target DB (%s): Connection Failed -> %v\n", cfg.Target.Driver, err)
				return fmt.Errorf("target database connectivity test failed")
			}
			fmt.Printf("  ✓ Target DB (%s): Connection Successful\n", cfg.Target.Driver)
		}
	}

	fmt.Println("\n✨ Configuration syntax and rules verified successfully!")
	return nil
}

func testPing(ctx context.Context, driver, dsn string) error {
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	driverName := driver
	if driverName == "postgres" || driverName == "postgresql" {
		driverName = "postgres"
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return err
	}
	defer db.Close()

	return db.PingContext(pingCtx)
}
