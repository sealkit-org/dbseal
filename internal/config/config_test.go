package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigWithEnvVars(t *testing.T) {
	os.Setenv("TEST_SOURCE_DB", "mysql://root:secret@127.0.0.1:3306/prod_db")
	defer os.Unsetenv("TEST_SOURCE_DB")

	yamlContent := `
version: "0.1"
source:
  driver: "mysql"
  dsn: "${TEST_SOURCE_DB}"
  options:
    throttle_sleep_ms: 15

subset:
  targets:
    - table: "users"
      query: "SELECT id FROM users LIMIT 100"

masking:
  tables:
    users:
      email: "faker.email"
      password: "constant.maskedpass"
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "dbseal.yaml")
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	if cfg.Source.DSN != "mysql://root:secret@127.0.0.1:3306/prod_db" {
		t.Errorf("expected expanded DSN, got %q", cfg.Source.DSN)
	}

	if cfg.Source.Options.ThrottleSleepMs != 15 {
		t.Errorf("expected throttle 15, got %d", cfg.Source.Options.ThrottleSleepMs)
	}

	if cfg.Masking.Tables["users"]["email"] != "faker.email" {
		t.Errorf("expected email masking rule, got %q", cfg.Masking.Tables["users"]["email"])
	}

	// Verify defaults
	if cfg.Subset.Strategy.MaxDepth != 4 {
		t.Errorf("expected default max_depth 4, got %d", cfg.Subset.Strategy.MaxDepth)
	}
}
