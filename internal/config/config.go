package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the complete dbseal configuration (v1.5).
type Config struct {
	Version   string          `yaml:"version"`
	Source    SourceConfig    `yaml:"source"`
	Target    TargetConfig    `yaml:"target,omitempty"`
	Relations RelationsConfig `yaml:"relations,omitempty"`
	Subset    SubsetConfig    `yaml:"subset,omitempty"`
	Masking            MaskingConfig   `yaml:"masking,omitempty"`
	Output             OutputConfig    `yaml:"output,omitempty"`
	VirtualForeignKeys []VirtualFKSpec `yaml:"virtual_foreign_keys,omitempty"`
}

type SourceConfig struct {
	Driver  string        `yaml:"driver"` // mysql | postgres
	DSN     string        `yaml:"dsn"`    // Connection string or ${ENV_VAR}
	Options SourceOptions `yaml:"options,omitempty"`
}

type SourceOptions struct {
	SSLMode         string `yaml:"ssl_mode,omitempty"`
	MaxOpenConns    int    `yaml:"max_open_conns,omitempty"`
	ThrottleSleepMs int    `yaml:"throttle_sleep_ms,omitempty"` // Sleep between batches (ms)
}

type TargetConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type RelationsConfig struct {
	AutoDetect AutoDetectConfig `yaml:"auto_detect,omitempty"`
	Virtual    []VirtualFK      `yaml:"virtual,omitempty"`
}

type AutoDetectConfig struct {
	Enabled   bool           `yaml:"enabled"`
	Patterns  []string       `yaml:"patterns,omitempty"`
	Sampling  SamplingConfig `yaml:"sampling,omitempty"`
}

type SamplingConfig struct {
	SampleRows     int     `yaml:"sample_rows,omitempty"`     // default 1000
	MatchThreshold float64 `yaml:"match_threshold,omitempty"` // default 0.95 (95%)
}

type VirtualFK struct {
	From      string `yaml:"from"` // e.g. orders.user_id
	To        string `yaml:"to"`   // e.g. users.id
	Type      string `yaml:"type"` // e.g. many-to-one
	Condition string `yaml:"condition,omitempty"`
}

type VirtualFKSpec struct {
	FromTable  string `yaml:"from_table"`
	FromColumn string `yaml:"from_column"`
	ToTable    string `yaml:"to_table"`
	ToColumn   string `yaml:"to_column"`
}

type SubsetConfig struct {
	Targets         []TargetQuery `yaml:"targets,omitempty"`
	ReferenceTables []string      `yaml:"reference_tables,omitempty"`
	Strategy        Strategy      `yaml:"strategy,omitempty"`
}

type TargetQuery struct {
	Table string `yaml:"table"`
	Query string `yaml:"query,omitempty"`
	Where string `yaml:"where,omitempty"`
	Limit int    `yaml:"limit,omitempty"`
}

// GetQuery returns the effective query, constructing one from Where and Limit if Query is empty.
func (t TargetQuery) GetQuery() string {
	if t.Query != "" {
		return t.Query
	}
	q := fmt.Sprintf("SELECT * FROM %s", t.Table)
	if t.Where != "" {
		q += " WHERE " + t.Where
	}
	if t.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", t.Limit)
	}
	return q
}

type Strategy struct {
	FollowDownstream     bool     `yaml:"follow_downstream"`
	CompleteDictionaries bool     `yaml:"complete_dictionaries"`
	MaxDepth             int      `yaml:"max_depth"`
	RowCap               int      `yaml:"row_cap"`
	RowCapExceeded       string   `yaml:"row_cap_exceeded"` // warn | fail
	ExcludeTables        []string `yaml:"exclude_tables,omitempty"`
}

// MaskingConfig maps table name -> column name -> rule (or nested json rule)
type MaskingConfig struct {
	Tables map[string]map[string]string `yaml:"tables,omitempty"`
}

// UnmarshalYAML supports both { tables: { users: ... } } and direct flat { users: ... }.
func (m *MaskingConfig) UnmarshalYAML(value *yaml.Node) error {
	type alias MaskingConfig
	var a alias
	if err := value.Decode(&a); err == nil && len(a.Tables) > 0 {
		*m = MaskingConfig(a)
		return nil
	}

	var flat map[string]map[string]string
	if err := value.Decode(&flat); err == nil {
		m.Tables = flat
		return nil
	}

	return nil
}

type OutputConfig struct {
	File        string `yaml:"file,omitempty"`
	Format      string `yaml:"format,omitempty"` // sql.gz | sql
	SelfHealing bool   `yaml:"self_healing,omitempty"`
}

// LoadConfig reads a YAML file, expands environment variables ${VAR}, and parses it into Config.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %q: %w", path, err)
	}

	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	setDefaults(&cfg)
	return &cfg, nil
}

// expandEnvVars expands both ${VAR} and $VAR patterns using os.Getenv.
func expandEnvVars(input string) string {
	re := regexp.MustCompile(`\$\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
	return re.ReplaceAllStringFunc(input, func(m string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(m, "${"), "}")
		return os.Getenv(varName)
	})
}

func setDefaults(cfg *Config) {
	if cfg.Source.Options.MaxOpenConns <= 0 {
		cfg.Source.Options.MaxOpenConns = 5
	}
	if cfg.Subset.Strategy.MaxDepth <= 0 {
		cfg.Subset.Strategy.MaxDepth = 4
	}
	if cfg.Subset.Strategy.RowCap <= 0 {
		cfg.Subset.Strategy.RowCap = 10000
	}
	if cfg.Subset.Strategy.RowCapExceeded == "" {
		cfg.Subset.Strategy.RowCapExceeded = "warn"
	}
	if cfg.Relations.AutoDetect.Sampling.SampleRows <= 0 {
		cfg.Relations.AutoDetect.Sampling.SampleRows = 1000
	}
	if cfg.Relations.AutoDetect.Sampling.MatchThreshold <= 0 {
		cfg.Relations.AutoDetect.Sampling.MatchThreshold = 0.95
	}
	if cfg.Output.File == "" {
		cfg.Output.File = "./dev_seed_data.sql.gz"
	}
	if cfg.Output.Format == "" {
		cfg.Output.Format = "sql.gz"
	}
}
