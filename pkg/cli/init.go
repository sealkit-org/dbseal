package cli

import (
	"fmt"
	"os"
)

const defaultYAMLTemplate = `version: "0.1"

# 1. Database Connections
# [Source] is REQUIRED by: dbseal slice, sync, and dump
source:
  driver: "mysql"           # mysql | postgres
  dsn: "${SOURCE_DB_URL}"   # MySQL:    root:password@tcp(127.0.0.1:3306)/prod_db?parseTime=true
                            # Postgres: postgres://user:password@127.0.0.1:5432/prod_db?sslmode=disable
  options:
    ssl_mode: "preferred"
    max_open_conns: 5
    throttle_sleep_ms: 10   # Micro-sleep between batches to protect production IOPS

# [Target] is REQUIRED ONLY by: dbseal sync (in-memory staging refresh)
# (Optional: leave target empty if you only generate offline files via 'dbseal slice')
target:
  driver: "mysql"           # mysql | postgres
  dsn: "${TARGET_DB_URL}"   # Test/staging database connection

# 2. Virtual Foreign Key Deduction & Sampling
relations:
  auto_detect:
    enabled: true           # Automatically infer {table}_id -> table.id
    sampling:
      sample_rows: 1000
      match_threshold: 0.95 # >= 95% confirmed as solid edge; below rejected to candidate

# 3. Subsetting Rules (Optional - omit for full database dump/sync)
subset:
  targets:
    - table: "users"
      query: "SELECT id FROM users WHERE status = 'active' ORDER BY id DESC LIMIT 500"

  # Reference tables preserved 100% regardless of graph connectivity
  reference_tables:
    - "system_settings"
    - "roles"
    - "permissions"

  strategy:
    follow_downstream: true     # Cascades 1:N relations (e.g. users -> orders)
    complete_dictionaries: true # Completes N:1 relations (e.g. orders -> users)
    max_depth: 4
    row_cap: 10000              # Maximum rows per table to prevent exploding
    row_cap_exceeded: "warn"    # warn | fail

# 4. Anonymization / Data Masking
masking:
  tables:
    users:
      email: "faker.email"
      phone: "faker.phone_number"
      password_hash: "constant.$2a$12$dummyhashforlocaldev..."
      # Use deterministic hash for relational/join columns to keep integrity:
      account_code: "hash.sha256"
    orders:
      shipping_address: "faker.address"

# 5. Output Configuration (REQUIRED by: dbseal slice and dump; IGNORED by: dbseal sync)
output:
  file: "./dev_seed_data.sql.gz"
  format: "sql.gz"
  self_healing: true          # Appends AUTO_INCREMENT reset and cycle resolution
`

// RunInit creates a fresh dbseal.yaml configuration file.
func RunInit(targetPath string, force bool) error {
	if targetPath == "" {
		targetPath = "dbseal.yaml"
	}

	if _, err := os.Stat(targetPath); err == nil && !force {
		return fmt.Errorf("configuration file %q already exists. Use -f or --force to overwrite", targetPath)
	}

	if err := os.WriteFile(targetPath, []byte(defaultYAMLTemplate), 0644); err != nil {
		return fmt.Errorf("failed to write %q: %w", targetPath, err)
	}

	fmt.Printf("✨ Created %q successfully!\n\n", targetPath)
	fmt.Println("Next steps:")
	fmt.Printf("  1. Edit %s to configure your database DSN and subset targets.\n", targetPath)
	if targetPath == "dbseal.yaml" {
		fmt.Println("  2. Run 'dbseal validate' to verify rules and connectivity.")
		fmt.Println("  3. Run 'dbseal slice' to generate your first self-healing development seed.")
		fmt.Println("  4. Run 'dbseal ui' to inspect your relational graph in your browser.")
	} else {
		fmt.Printf("  2. Run 'dbseal validate %s --ping' to verify rules and connectivity.\n", targetPath)
		fmt.Printf("  3. Run 'dbseal slice --config %s' to generate your first self-healing development seed.\n", targetPath)
		fmt.Printf("  4. Run 'dbseal ui --config %s' to inspect your relational graph in your browser.\n", targetPath)
	}

	return nil
}
