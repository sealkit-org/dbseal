package config

import (
	"testing"
)

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := &Config{
		Version: "0.1",
		Source: SourceConfig{
			Driver: "mysql",
			DSN:    "root:pass@tcp(127.0.0.1:3306)/test",
		},
		Subset: SubsetConfig{
			Targets: []TargetQuery{
				{Table: "users", Query: "SELECT id FROM users LIMIT 10"},
			},
		},
		Masking: MaskingConfig{
			Tables: map[string]map[string]string{
				"users": {
					"email": "faker.email",
					"phone": "mask.keep_first_last",
				},
			},
		},
		Output: OutputConfig{
			File: "./dev_seed.sql.gz",
		},
	}

	report := cfg.Validate("")
	if report.HasError {
		t.Fatalf("expected no errors for valid config, got: %+v", report.Issues)
	}

	if !report.SliceReady {
		t.Errorf("expected slice to be ready")
	}

	// Without target DSN, sync is not ready
	if report.SyncReady {
		t.Errorf("expected sync to NOT be ready without target DSN")
	}

	// In sync mode, missing target DSN should be an error
	syncReport := cfg.Validate("sync")
	if !syncReport.HasError {
		t.Errorf("expected error in sync mode when target DSN is missing")
	}
}

func TestConfig_Validate_Errors(t *testing.T) {
	cfg := &Config{
		Source: SourceConfig{
			Driver: "oracle", // invalid driver
			DSN:    "",       // empty DSN
		},
		Subset: SubsetConfig{
			Targets: []TargetQuery{
				{Table: "", Query: ""}, // missing table and query
			},
			Strategy: Strategy{
				MaxDepth: -1, // negative depth
			},
		},
	}

	report := cfg.Validate("")
	if !report.HasError {
		t.Fatalf("expected errors for invalid config, got none")
	}

	if len(report.Issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d", len(report.Issues))
	}
}
