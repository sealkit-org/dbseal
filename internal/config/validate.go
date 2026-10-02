package config

import (
	"fmt"
	"strings"
)

// ValidationIssue represents a warning or error discovered during validation.
type ValidationIssue struct {
	Level   string // "ERROR" or "WARN"
	Section string // "source", "target", "subset", "masking", etc.
	Message string
}

// ValidationReport holds the result of validating a Config.
type ValidationReport struct {
	Path        string
	Version     string
	Issues      []ValidationIssue
	HasError    bool
	SliceReady  bool
	SliceReason string
	SyncReady   bool
	SyncReason  string
}

// AddError records an error-level validation failure.
func (r *ValidationReport) AddError(section, msg string) {
	r.Issues = append(r.Issues, ValidationIssue{
		Level:   "ERROR",
		Section: section,
		Message: msg,
	})
	r.HasError = true
}

// AddWarn records a warning-level validation issue.
func (r *ValidationReport) AddWarn(section, msg string) {
	r.Issues = append(r.Issues, ValidationIssue{
		Level:   "WARN",
		Section: section,
		Message: msg,
	})
}

// Validate checks the configuration for structural correctness and rule validity.
func (c *Config) Validate(mode string) *ValidationReport {
	report := &ValidationReport{
		Version: c.Version,
	}

	// 1. Version check
	if c.Version == "" {
		report.AddWarn("version", "version field is omitted; defaulting to '0.1'")
	}

	// 2. Source database checks
	if c.Source.Driver == "" {
		report.AddError("source.driver", "source driver is required (e.g. 'mysql' or 'postgres')")
	} else {
		driver := strings.ToLower(c.Source.Driver)
		if driver != "mysql" && driver != "postgres" && driver != "postgresql" {
			report.AddError("source.driver", fmt.Sprintf("unsupported driver %q; supported: 'mysql', 'postgres'", c.Source.Driver))
		}
	}

	if strings.TrimSpace(c.Source.DSN) == "" {
		report.AddError("source.dsn", "source DSN connection string is empty (check if environment variable was loaded)")
	}

	// 3. Target database checks
	targetSpecified := strings.TrimSpace(c.Target.DSN) != ""
	if strings.ToLower(mode) == "sync" && !targetSpecified {
		report.AddError("target.dsn", "target database DSN is required when running in 'sync' mode (configure target.dsn)")
	}

	if c.Target.Driver != "" || targetSpecified {
		if c.Target.Driver == "" {
			report.AddError("target.driver", "target driver is required when target DSN is specified")
		} else {
			driver := strings.ToLower(c.Target.Driver)
			if driver != "mysql" && driver != "postgres" && driver != "postgresql" {
				report.AddError("target.driver", fmt.Sprintf("unsupported target driver %q; supported: 'mysql', 'postgres'", c.Target.Driver))
			}
		}
	}

	// 4. Subsetting checks
	for i, t := range c.Subset.Targets {
		if strings.TrimSpace(t.Table) == "" {
			report.AddError("subset.targets", fmt.Sprintf("target query #%d is missing table name", i+1))
		}
		if strings.TrimSpace(t.Query) == "" && strings.TrimSpace(t.Where) == "" {
			report.AddError("subset.targets", fmt.Sprintf("target query #%d for table %q is missing query SQL or where filter", i+1, t.Table))
		}
	}

	if c.Subset.Strategy.MaxDepth < 0 {
		report.AddError("subset.strategy.max_depth", "max_depth cannot be negative")
	}
	if c.Subset.Strategy.RowCap < 0 {
		report.AddError("subset.strategy.row_cap", "row_cap cannot be negative")
	}

	// 5. Masking checks
	validMaskPrefixes := []string{"faker.", "mask.", "hash.", "constant.", "null", "none", "jsonpath", "hmac"}
	for table, cols := range c.Masking.Tables {
		if len(cols) == 0 {
			report.AddWarn("masking.tables", fmt.Sprintf("table %q has empty masking configuration", table))
		}
		for col, rule := range cols {
			if strings.TrimSpace(rule) == "" {
				report.AddWarn("masking.tables", fmt.Sprintf("column %s.%s has empty masking rule", table, col))
				continue
			}
			matched := false
			for _, prefix := range validMaskPrefixes {
				if strings.HasPrefix(rule, prefix) || rule == "null" || rule == "none" {
					matched = true
					break
				}
			}
			if !matched {
				report.AddWarn("masking.tables", fmt.Sprintf("column %s.%s has unrecognized masking rule %q (custom or typo?)", table, col, rule))
			}
		}
	}

	// 6. Output checks
	if c.Output.Format != "" {
		fmtLower := strings.ToLower(c.Output.Format)
		if fmtLower != "sql.gz" && fmtLower != "sql" {
			report.AddWarn("output.format", fmt.Sprintf("unusual output format %q; standard formats are 'sql.gz' and 'sql'", c.Output.Format))
		}
	}

	// 7. Calculate Mode Readiness
	sourceValid := c.Source.Driver != "" && strings.TrimSpace(c.Source.DSN) != ""
	report.SliceReady = sourceValid && c.Output.File != "" && !report.HasError
	if report.SliceReady {
		report.SliceReason = fmt.Sprintf("READY -> exports snapshot to %s", c.Output.File)
	} else {
		report.SliceReason = "NOT READY -> configuration errors present"
	}

	report.SyncReady = sourceValid && targetSpecified && !report.HasError
	if report.SyncReady {
		report.SyncReason = fmt.Sprintf("READY -> streaming into target DB (%s)", c.Target.Driver)
	} else if !targetSpecified {
		report.SyncReason = "UNCONFIGURED -> target.dsn is empty (required only if running 'dbseal sync')"
	} else {
		report.SyncReason = "NOT READY -> configuration errors present"
	}

	return report
}
