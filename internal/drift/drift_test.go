package drift

import (
	"context"
	"reflect"
	"testing"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
)

type MockDriftDriver struct {
	schemas map[string]*drivers.TableSchema
}

func (m *MockDriftDriver) Name() string { return "mock" }
func (m *MockDriftDriver) Open(ctx context.Context, dsn string, opts config.SourceOptions) error { return nil }
func (m *MockDriftDriver) Close() error { return nil }
func (m *MockDriftDriver) Ping(ctx context.Context) error { return nil }
func (m *MockDriftDriver) GetTables(ctx context.Context) ([]string, error) { return nil, nil }
func (m *MockDriftDriver) GetTableEstimatedRows(ctx context.Context, table string) (int64, error) { return 0, nil }
func (m *MockDriftDriver) GetForeignKeys(ctx context.Context) ([]drivers.ForeignKey, error) { return nil, nil }
func (m *MockDriftDriver) GetTableSchema(ctx context.Context, table string) (*drivers.TableSchema, error) {
	return m.schemas[table], nil
}
func (m *MockDriftDriver) QueryIDs(ctx context.Context, query string) ([]any, error) { return nil, nil }
func (m *MockDriftDriver) QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error) { return nil, nil }
func (m *MockDriftDriver) StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback drivers.RowCallback) error { return nil }
func (m *MockDriftDriver) FormatInsertSQL(table string, columns []string, rows [][]any) (string, error) { return "", nil }
func (m *MockDriftDriver) SessionHeaderSQL() string { return "" }
func (m *MockDriftDriver) SessionFooterSQL() string { return "" }
func (m *MockDriftDriver) ExecuteSQL(ctx context.Context, sql string) error { return nil }
func (m *MockDriftDriver) WipeTable(ctx context.Context, table string) error { return nil }
func (m *MockDriftDriver) ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error { return nil }
func (m *MockDriftDriver) SetForeignKeyChecks(ctx context.Context, enabled bool) error { return nil }

func TestAnalyzeDriftAndProjectRow(t *testing.T) {
	// Source (Production):
	// id, username, email, old_deprecated_column
	sourceDriver := &MockDriftDriver{
		schemas: map[string]*drivers.TableSchema{
			"users": {
				TableName: "users",
				Columns: []drivers.ColumnInfo{
					{Name: "id", IsPrimaryKey: true, IsAutoIncr: true},
					{Name: "username"},
					{Name: "email"},
					{Name: "old_deprecated_column"},
				},
			},
		},
	}

	// Target (Staging with new migration):
	// id, username, email, new_field_nullable, new_field_required
	targetDriver := &MockDriftDriver{
		schemas: map[string]*drivers.TableSchema{
			"users": {
				TableName: "users",
				Columns: []drivers.ColumnInfo{
					{Name: "id", IsPrimaryKey: true, IsAutoIncr: true},
					{Name: "username"},
					{Name: "email"},
					{Name: "new_field_nullable", IsNullable: true},
					{Name: "new_field_required", IsNullable: false, DefaultValue: nil}, // Missing not null no default
				},
			},
		},
	}

	adapter := NewAdapter(sourceDriver, targetDriver)
	res, err := adapter.AnalyzeDrift(context.Background(), "users")
	if err != nil {
		t.Fatalf("unexpected error analyzing drift: %v", err)
	}

	// 1. Common columns intersection: id, username, email
	expectedCommon := []string{"id", "username", "email"}
	if !reflect.DeepEqual(res.CommonCols, expectedCommon) {
		t.Errorf("expected common cols %v, got %v", expectedCommon, res.CommonCols)
	}

	// 2. Source-only deprecated column: old_deprecated_column
	if len(res.SourceOnlyCols) != 1 || res.SourceOnlyCols[0] != "old_deprecated_column" {
		t.Errorf("expected source-only 'old_deprecated_column', got %v", res.SourceOnlyCols)
	}

	// 3. Target-only columns
	if len(res.TargetOnlyCols) != 2 {
		t.Errorf("expected 2 target-only cols, got %v", res.TargetOnlyCols)
	}

	// 4. Missing NOT NULL without DEFAULT: new_field_required
	if len(res.MissingNotNullNoDefault) != 1 || res.MissingNotNullNoDefault[0] != "new_field_required" {
		t.Errorf("expected missing required col 'new_field_required', got %v", res.MissingNotNullNoDefault)
	}

	// 5. Test ProjectRow projection
	sourceRow := []any{int64(101), "alice", "alice@example.com", "old_legacy_val"}
	sourceCols := []string{"id", "username", "email", "old_deprecated_column"}
	projected := ProjectRow(sourceRow, sourceCols, res.CommonCols)

	expectedProjected := []any{int64(101), "alice", "alice@example.com"}
	if !reflect.DeepEqual(projected, expectedProjected) {
		t.Errorf("expected projected row %v, got %v", expectedProjected, projected)
	}
}
