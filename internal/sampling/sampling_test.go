package sampling

import (
	"context"
	"strings"
	"testing"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
)

func TestToSingularAndPossibleNames(t *testing.T) {
	cases := map[string]string{
		"users":      "user",
		"categories": "category",
		"companies":  "company",
		"addresses":  "address",
		"orders":     "order",
	}

	for plural, expected := range cases {
		actual := ToSingular(plural)
		if actual != expected {
			t.Errorf("ToSingular(%q) = %q, expected %q", plural, actual, expected)
		}
	}

	names := PossibleFKColumnNames("users")
	foundUserID := false
	foundCreatedBy := false
	for _, n := range names {
		if n == "user_id" {
			foundUserID = true
		}
		if n == "created_by" {
			foundCreatedBy = true
		}
	}
	if !foundUserID || !foundCreatedBy {
		t.Errorf("expected user_id and created_by in possible names, got %v", names)
	}
}

type MockSamplingDriver struct {
	schemas map[string]*drivers.TableSchema
	mockIDs map[string][]any
}

func (m *MockSamplingDriver) Name() string { return "mock" }
func (m *MockSamplingDriver) Open(ctx context.Context, dsn string, opts config.SourceOptions) error { return nil }
func (m *MockSamplingDriver) Close() error { return nil }
func (m *MockSamplingDriver) Ping(ctx context.Context) error { return nil }
func (m *MockSamplingDriver) GetTables(ctx context.Context) ([]string, error) { return nil, nil }
func (m *MockSamplingDriver) GetTableEstimatedRows(ctx context.Context, table string) (int64, error) { return 0, nil }
func (m *MockSamplingDriver) GetForeignKeys(ctx context.Context) ([]drivers.ForeignKey, error) { return nil, nil }
func (m *MockSamplingDriver) GetTableSchema(ctx context.Context, table string) (*drivers.TableSchema, error) {
	return m.schemas[table], nil
}
func (m *MockSamplingDriver) QueryIDs(ctx context.Context, query string) ([]any, error) {
	for k, ids := range m.mockIDs {
		if strings.Contains(query, k) {
			return ids, nil
		}
	}
	return nil, nil
}
func (m *MockSamplingDriver) QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error) {
	return nil, nil
}
func (m *MockSamplingDriver) StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback drivers.RowCallback) error {
	return nil
}
func (m *MockSamplingDriver) FormatInsertSQL(table string, columns []string, rows [][]any) (string, error) { return "", nil }
func (m *MockSamplingDriver) SessionHeaderSQL() string { return "" }
func (m *MockSamplingDriver) SessionFooterSQL() string { return "" }
func (m *MockSamplingDriver) ExecuteSQL(ctx context.Context, sql string) error { return nil }
func (m *MockSamplingDriver) WipeTable(ctx context.Context, table string) error { return nil }
func (m *MockSamplingDriver) ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error { return nil }
func (m *MockSamplingDriver) SetForeignKeyChecks(ctx context.Context, enabled bool) error { return nil }

func TestSamplingValidationGrading(t *testing.T) {
	// Case 1: High match rate (confirmed)
	mockHigh := &MockSamplingDriver{
		mockIDs: map[string][]any{
			"SELECT DISTINCT `user_id` FROM `orders`": {int64(1), int64(2), int64(3), int64(4), int64(5)}, // 5 sampled
			"SELECT `id` FROM `users`":                {int64(1), int64(2), int64(3), int64(4), int64(5)}, // 5 matched (100%)
		},
	}
	v1 := NewValidator(mockHigh, &config.Config{})
	res1, err := v1.ValidateRelation(context.Background(), InferredRelation{
		FromTable:  "orders",
		FromColumn: "user_id",
		ToTable:    "users",
		ToColumn:   "id",
	}, 1000, 0.95)

	if err != nil || res1.Status != "confirmed" || res1.MatchRate != 1.0 {
		t.Fatalf("expected confirmed with 1.0 match rate, got %+v", res1)
	}

	// Case 2: Low match rate (candidate, rejected from auto-traversal)
	mockLow := &MockSamplingDriver{
		mockIDs: map[string][]any{
			"SELECT DISTINCT `app_id` FROM `logs`": {int64(10), int64(20), int64(30), int64(40), int64(50)}, // 5 sampled
			"SELECT `id` FROM `apps`":              {int64(10), int64(20)},                                  // 2 matched (40%)
		},
	}
	v2 := NewValidator(mockLow, &config.Config{})
	res2, err := v2.ValidateRelation(context.Background(), InferredRelation{
		FromTable:  "logs",
		FromColumn: "app_id",
		ToTable:    "apps",
		ToColumn:   "id",
	}, 1000, 0.95)

	if err != nil || res2.Status != "candidate" || res2.MatchRate != 0.4 {
		t.Fatalf("expected candidate with 0.4 match rate, got %+v", res2)
	}

	// Case 3: Empty child table (unverified)
	mockEmpty := &MockSamplingDriver{
		mockIDs: map[string][]any{},
	}
	v3 := NewValidator(mockEmpty, &config.Config{})
	res3, err := v3.ValidateRelation(context.Background(), InferredRelation{
		FromTable:  "empty_table",
		FromColumn: "user_id",
		ToTable:    "users",
		ToColumn:   "id",
	}, 1000, 0.95)

	if err != nil || res3.Status != "unverified" {
		t.Fatalf("expected unverified for empty table, got %+v", res3)
	}
}
