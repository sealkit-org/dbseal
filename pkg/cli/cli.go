package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/sealkit/dbseal/internal/config"
	_ "github.com/sealkit/dbseal/internal/drivers/mysql"    // register MySQL dialect
	_ "github.com/sealkit/dbseal/internal/drivers/postgres" // register PostgreSQL dialect
	"github.com/sealkit/dbseal/internal/pipeline"
	"github.com/sealkit/dbseal/internal/ui"
	"github.com/sealkit/dbseal/pkg/extension"
)

// Execute runs the CLI application with os.Args.
func Execute() {
	if err := Run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nError: %v\n", err)
		os.Exit(1)
	}
}

// Run executes CLI with the given context and argument slice.
func Run(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		PrintUsage()
		return nil
	}

	cmd := args[0]
	cmdArgs := args[1:]

	switch cmd {
	case "dump":
		return runDump(ctx, cmdArgs)
	case "sync":
		return runSync(ctx, cmdArgs)
	case "slice":
		return runSlice(ctx, cmdArgs)
	case "restore":
		return runRestore(ctx, cmdArgs)
	case "ui":
		return runUI(ctx, cmdArgs)
	case "init":
		return runInit(cmdArgs)
	case "validate":
		return runValidate(ctx, cmdArgs)
	case "version", "-v", "--version":
		PrintVersion()
		return nil
	default:
		PrintUsage()
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func runDump(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("dump", flag.ContinueOnError)
	configPath := fs.String("config", "dbseal.yaml", "Path to dbseal YAML configuration")
	outputPath := fs.String("output", "", "Output file path (overrides config output.file)")
	tablesFlag := fs.String("tables", "", "Comma-separated list of tables to dump (optional)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config %q: %w", *configPath, err)
	}

	var explicitTables []string
	if *tablesFlag != "" {
		for _, t := range strings.Split(*tablesFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				explicitTables = append(explicitTables, t)
			}
		}
	}

	opts := pipeline.DumpOptions{
		ConfigPath: *configPath,
		OutputPath: *outputPath,
		Tables:     explicitTables,
	}

	return pipeline.ExecuteDump(ctx, cfg, opts)
}

func runSync(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	configPath := fs.String("config", "dbseal.yaml", "Path to dbseal YAML configuration")
	tablesFlag := fs.String("tables", "", "Comma-separated list of tables to sync (optional)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config %q: %w", *configPath, err)
	}

	var explicitTables []string
	if *tablesFlag != "" {
		for _, t := range strings.Split(*tablesFlag, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				explicitTables = append(explicitTables, t)
			}
		}
	}

	opts := pipeline.SyncOptions{
		ConfigPath: *configPath,
		Tables:     explicitTables,
	}

	return pipeline.ExecuteSync(ctx, cfg, opts)
}

func runSlice(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("slice", flag.ContinueOnError)
	configPath := fs.String("config", "dbseal.yaml", "Path to dbseal YAML configuration")
	outputPath := fs.String("output", "", "Output file path (overrides config output.file)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return fmt.Errorf("failed to load config %q: %w", *configPath, err)
	}

	opts := pipeline.SliceOptions{
		ConfigPath: *configPath,
		OutputPath: *outputPath,
	}

	return pipeline.ExecuteSlice(ctx, cfg, opts)
}

func runRestore(ctx context.Context, args []string) error {
	fmt.Println("[dbseal restore] Restoring slice file into target database...")
	return nil
}

func runUI(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("ui", flag.ContinueOnError)
	port := fs.Int("port", 8080, "Port for local web server")
	noBrowser := fs.Bool("no-browser", false, "Do not open browser automatically")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	return ui.Start(ctx, ui.ServerOptions{
		Port:      *port,
		NoBrowser: *noBrowser,
	})
}

func printInitUsage() {
	fmt.Println("dbseal init 🦭 - Create a starter configuration file")
	fmt.Println("\nUsage:")
	fmt.Println("  dbseal init [filename] [flags]")
	fmt.Println("\nArguments:")
	fmt.Println("  filename      Optional configuration file path (default: dbseal.yaml)")
	fmt.Println("                If extension is omitted, .yaml will be added automatically")
	fmt.Println("\nFlags:")
	fmt.Println("  -f, --force   Overwrite configuration file if it already exists")
	fmt.Println("  -h, --help    Show help for init command")
	fmt.Println("\nExamples:")
	fmt.Println("  dbseal init               # Creates dbseal.yaml")
	fmt.Println("  dbseal init staging       # Creates staging.yaml")
	fmt.Println("  dbseal init orders.yaml   # Creates orders.yaml")
	fmt.Println("  dbseal init -f            # Overwrites existing configuration file")
}

func runInit(args []string) error {
	force := false
	target := ""

	for _, arg := range args {
		switch arg {
		case "-h", "--help":
			printInitUsage()
			return nil
		case "-f", "--force":
			force = true
		default:
			if !strings.HasPrefix(arg, "-") && target == "" {
				target = arg
			}
		}
	}

	if target == "" {
		target = "dbseal.yaml"
	}

	// Auto-append .yaml extension if omitted
	if !strings.HasSuffix(target, ".yaml") && !strings.HasSuffix(target, ".yml") {
		target = target + ".yaml"
	}

	return RunInit(target, force)
}

func PrintVersion() {
	p := extension.GetPipeline()
	pName := "oss-single-stream"
	if p != nil {
		pName = p.Name()
	}
	fmt.Printf("dbseal v0.1.0 (%s)\n", pName)
	fmt.Printf("Local-first Database Subsetting & Masking Toolkit\n")
}

func PrintUsage() {
	fmt.Println("dbseal 🦭 - Local-first Database Subsetting & Masking Toolkit")
	fmt.Println("\nUsage:")
	fmt.Println("  dbseal <command> [flags]")
	fmt.Println("\nAvailable Commands:")
	fmt.Println("  init [path]      Create a starter YAML configuration file (e.g. dbseal init staging.yaml)")
	fmt.Println("  validate [path]  Verify configuration syntax, rules, and database connectivity")
	fmt.Println("  slice            Extract relation-consistent subset with self-healing SQL")
	fmt.Println("  sync             Direct in-memory staging refresh (Source -> Target)")
	fmt.Println("  dump             Full database streaming anonymized export (.sql.gz)")
	fmt.Println("  restore          Restore slice file into target test database")
	fmt.Println("  ui               Launch interactive React Flow Web UI (127.0.0.1:8080)")
	fmt.Println("  version          Print version information")
	fmt.Println("\nExamples:")
	fmt.Println("  dbseal init")
	fmt.Println("  dbseal validate")
	fmt.Println("  dbseal validate staging.yaml --ping")
	fmt.Println("  dbseal slice --config staging.yaml --output ./dev_seed.sql.gz")
	fmt.Println("  dbseal sync --config dbseal.yaml")
	fmt.Println("  dbseal ui --port 8080")
}
