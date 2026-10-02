package postgres

import (
	"strings"
	"testing"
)

func TestFormatPGInsertSQL(t *testing.T) {
	d := &PostgresDriver{}
	cols := []string{"id", "username", "is_active", "bio"}
	rows := [][]any{
		{int64(1), "bob", true, "Software Engineer's log"},
		{int64(2), "alice", false, nil},
	}

	sql, err := d.FormatInsertSQL("users", cols, rows)
	if err != nil {
		t.Fatalf("failed to format PG insert SQL: %v", err)
	}

	expectedHeader := `INSERT INTO "users" ("id", "username", "is_active", "bio") OVERRIDING SYSTEM VALUE VALUES`
	if !strings.Contains(sql, expectedHeader) {
		t.Errorf("expected header %q, got %q", expectedHeader, sql)
	}

	// Verify quote escaping for O'Reilly
	if !strings.Contains(sql, "Software Engineer''s log") {
		t.Errorf("expected single quote escaping in PG, got %s", sql)
	}

	// Verify boolean TRUE/FALSE in PG
	if !strings.Contains(sql, "TRUE") || !strings.Contains(sql, "FALSE") {
		t.Errorf("expected TRUE/FALSE literals in PG, got %s", sql)
	}
}
