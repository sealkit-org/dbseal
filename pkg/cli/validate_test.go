package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRunValidate_Success(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "valid.yaml")
	content := `
version: "0.1"
source:
  driver: "mysql"
  dsn: "root:password@tcp(127.0.0.1:3306)/db"
subset:
  targets:
    - table: "users"
      query: "SELECT id FROM users LIMIT 10"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := Run(context.Background(), []string{"validate", configPath})
	if err != nil {
		t.Fatalf("expected validation success, got error: %v", err)
	}
}

func TestRunValidate_SyntaxError(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid.yaml")
	content := `
version: "0.1"
source:
  driver: "oracle"
  dsn: ""
subset:
  targets:
    - table: ""
      query: ""
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	err := Run(context.Background(), []string{"validate", configPath})
	if err == nil {
		t.Fatalf("expected validation failure for invalid config, got nil")
	}
}

func TestRunValidate_Help(t *testing.T) {
	err := Run(context.Background(), []string{"validate", "-h"})
	if err != nil {
		t.Fatalf("expected nil error for help, got: %v", err)
	}
}

func TestRunValidate_WithFlags(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "custom_config.yaml")
	content := `
version: "0.1"
source:
  driver: "mysql"
  dsn: "root:password@tcp(127.0.0.1:3306)/db"
subset:
  targets:
    - table: "users"
      query: "SELECT id FROM users LIMIT 10"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Test with --config
	err := Run(context.Background(), []string{"validate", "--config", configPath, "--mode", "slice"})
	if err != nil {
		t.Fatalf("expected success with --config, got: %v", err)
	}

	// Test with -c and --mode=slice
	err = Run(context.Background(), []string{"validate", "-c", configPath, "--mode=slice"})
	if err != nil {
		t.Fatalf("expected success with -c, got: %v", err)
	}
}
