package drivers

import (
	"context"
	"fmt"
	"sync"

	"github.com/sealkit/dbseal/internal/config"
)

// ColumnInfo describes a single column's metadata.
type ColumnInfo struct {
	Name          string
	DataType      string
	IsNullable    bool
	IsPrimaryKey  bool
	IsGenerated   bool // VIRTUAL or STORED generated column
	IsAutoIncr    bool
	DefaultValue  *string
	Extra         string
}

// TableSchema describes a table's complete column layout.
type TableSchema struct {
	TableName string
	Columns   []ColumnInfo
	PKColumns []string
}

// GetInsertableColumns returns columns that can be explicitly written (excluding generated columns).
func (ts *TableSchema) GetInsertableColumns() []string {
	var cols []string
	for _, c := range ts.Columns {
		if !c.IsGenerated {
			cols = append(cols, c.Name)
		}
	}
	return cols
}

// RowCallback is invoked for each batch of streamed rows.
type RowCallback func(batch [][]any) error

// ForeignKey describes a foreign key relationship between two tables.
type ForeignKey struct {
	ConstraintName string
	FromTable      string
	FromColumn     string
	ToTable        string
	ToColumn       string
}

// Driver abstracts database dialects (MySQL, PostgreSQL, etc.).
type Driver interface {
	Name() string
	Open(ctx context.Context, dsn string, opts config.SourceOptions) error
	Close() error
	Ping(ctx context.Context) error

	// Schema & FK inspection
	GetTables(ctx context.Context) ([]string, error)
	GetTableSchema(ctx context.Context, table string) (*TableSchema, error)
	GetTableEstimatedRows(ctx context.Context, table string) (int64, error)
	GetForeignKeys(ctx context.Context) ([]ForeignKey, error)

	// Subsetting Queries
	QueryIDs(ctx context.Context, query string) ([]any, error)
	QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error)

	// Stream reading
	StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback RowCallback) error

	// SQL generation
	FormatInsertSQL(table string, columns []string, rows [][]any) (string, error)
	SessionHeaderSQL() string
	SessionFooterSQL() string

	// Target execution & wipe
	ExecuteSQL(ctx context.Context, sql string) error
	WipeTable(ctx context.Context, table string) error
	ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error
	SetForeignKeyChecks(ctx context.Context, enabled bool) error
}

var (
	registryMu sync.RWMutex
	drivers    = make(map[string]Driver)
)

// Register registers a database driver implementation.
func Register(name string, d Driver) {
	registryMu.Lock()
	defer registryMu.Unlock()
	drivers[name] = d
}

// Get returns the driver registered under name.
func Get(name string) (Driver, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("database driver %q not supported or registered", name)
	}
	return d, nil
}
