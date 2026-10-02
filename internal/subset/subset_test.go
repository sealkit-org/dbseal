package subset

import (
	"context"
	"strings"
	"testing"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
	"github.com/sealkit/dbseal/internal/graph"
)

// MockDriver implements minimal drivers.Driver for subsetting tests.
type MockDriver struct {
	schemas map[string]*drivers.TableSchema
	mockIDs map[string][]any
}

func (m *MockDriver) Name() string { return "mock" }
func (m *MockDriver) Open(ctx context.Context, dsn string, opts config.SourceOptions) error { return nil }
func (m *MockDriver) Close() error { return nil }
func (m *MockDriver) Ping(ctx context.Context) error { return nil }
func (m *MockDriver) GetTables(ctx context.Context) ([]string, error) {
	return []string{"users", "orders", "roles", "settings"}, nil
}
func (m *MockDriver) GetTableEstimatedRows(ctx context.Context, table string) (int64, error) {
	return 100, nil
}
func (m *MockDriver) GetForeignKeys(ctx context.Context) ([]drivers.ForeignKey, error) {
	return []drivers.ForeignKey{
		{ConstraintName: "fk_orders_user", FromTable: "orders", FromColumn: "user_id", ToTable: "users", ToColumn: "id"},
	}, nil
}
func (m *MockDriver) GetTableSchema(ctx context.Context, table string) (*drivers.TableSchema, error) {
	if s, ok := m.schemas[table]; ok {
		return s, nil
	}
	return &drivers.TableSchema{TableName: table, PKColumns: []string{"id"}}, nil
}
func (m *MockDriver) QueryIDs(ctx context.Context, query string) ([]any, error) {
	// Simple matching based on query keywords
	for key, ids := range m.mockIDs {
		if strings.Contains(query, key) {
			return ids, nil
		}
	}
	return nil, nil
}
func (m *MockDriver) QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error) {
	return nil, nil
}
func (m *MockDriver) StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback drivers.RowCallback) error {
	return nil
}
func (m *MockDriver) FormatInsertSQL(table string, columns []string, rows [][]any) (string, error) {
	return "", nil
}
func (m *MockDriver) SessionHeaderSQL() string { return "" }
func (m *MockDriver) SessionFooterSQL() string { return "" }
func (m *MockDriver) ExecuteSQL(ctx context.Context, sql string) error { return nil }
func (m *MockDriver) WipeTable(ctx context.Context, table string) error { return nil }
func (m *MockDriver) ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error { return nil }
func (m *MockDriver) SetForeignKeyChecks(ctx context.Context, enabled bool) error { return nil }

func TestSubsettingPlanExecution(t *testing.T) {
	mock := &MockDriver{
		schemas: map[string]*drivers.TableSchema{
			"users":  {TableName: "users", PKColumns: []string{"id"}},
			"orders": {TableName: "orders", PKColumns: []string{"id"}},
			"roles":  {TableName: "roles", PKColumns: []string{"id"}},
		},
		mockIDs: map[string][]any{
			"SELECT id FROM users": {int64(101), int64(102)},
			"SELECT `id` FROM `orders`": {int64(5001), int64(5002), int64(5003)},
		},
	}

	g := graph.NewGraph()
	g.AddRelationship(graph.Relationship{
		FromTable:  "orders",
		FromColumn: "user_id",
		ToTable:    "users",
		ToColumn:   "id",
	})

	cfg := &config.Config{
		Subset: config.SubsetConfig{
			Targets: []config.TargetQuery{
				{Table: "users", Query: "SELECT id FROM users WHERE status = 'active' LIMIT 10"},
			},
			ReferenceTables: []string{"roles", "settings"},
			Strategy: config.Strategy{
				FollowDownstream:     true,
				CompleteDictionaries: true,
				MaxDepth:             3,
				RowCap:               100,
			},
		},
	}

	engine := NewEngine(mock, cfg, g)
	plan, err := engine.BuildPlan(context.Background())
	if err != nil {
		t.Fatalf("failed to build subsetting plan: %v", err)
	}

	// 1. Verify Reference Tables 100% preservation
	if !plan.ReferenceTables["roles"] || !plan.ReferenceTables["settings"] {
		t.Errorf("expected reference tables 'roles' and 'settings' registered, got: %+v", plan.ReferenceTables)
	}

	// 2. Verify Root Seed Table IDs
	if len(plan.TablePKs["users"]) != 2 {
		t.Errorf("expected 2 users, got %d (%v)", len(plan.TablePKs["users"]), plan.TablePKs["users"])
	}

	// 3. Verify Downstream 1:N Cascade to Orders
	if len(plan.TablePKs["orders"]) != 3 {
		t.Errorf("expected 3 cascaded orders, got %d (%v)", len(plan.TablePKs["orders"]), plan.TablePKs["orders"])
	}
}

func TestSubsettingRowCap(t *testing.T) {
	mock := &MockDriver{
		schemas: map[string]*drivers.TableSchema{
			"users": {TableName: "users", PKColumns: []string{"id"}},
			"logs":  {TableName: "logs", PKColumns: []string{"id"}},
		},
		mockIDs: map[string][]any{
			"users": {int64(1)},
			"logs":  {int64(10), int64(11), int64(12), int64(13), int64(14)}, // 5 rows
		},
	}

	g := graph.NewGraph()
	g.AddRelationship(graph.Relationship{FromTable: "logs", FromColumn: "user_id", ToTable: "users", ToColumn: "id"})

	cfg := &config.Config{
		Subset: config.SubsetConfig{
			Targets: []config.TargetQuery{{Table: "users", Query: "SELECT id FROM users"}},
			Strategy: config.Strategy{
				FollowDownstream: true,
				RowCap:           3, // Cap at 3 rows
				RowCapExceeded:   "warn",
			},
		},
	}

	engine := NewEngine(mock, cfg, g)
	plan, err := engine.BuildPlan(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be capped at 3 rows
	if len(plan.TablePKs["logs"]) > 3 {
		t.Errorf("expected logs capped at 3, got %d", len(plan.TablePKs["logs"]))
	}

	// Warning should be recorded
	if len(plan.TableWarnings["logs"]) == 0 {
		t.Errorf("expected row_cap warning on logs table, got none")
	}
}
