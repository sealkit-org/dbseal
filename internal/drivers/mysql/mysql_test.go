package mysql

import (
	"strings"
	"testing"
)

func TestEscapeMySQLString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{`hello`, `hello`},
		{`O'Reilly`, `O\'Reilly`},
		{`He said "hello"`, `He said \"hello\"`},
		{"line1\nline2", `line1\nline2`},
		{"line1\rline2", `line1\rline2`},
		{`C:\Users\path`, `C:\\Users\\path`},
	}

	for _, c := range cases {
		actual := escapeMySQLString(c.input)
		if actual != c.expected {
			t.Errorf("escapeMySQLString(%q) = %q, expected %q", c.input, actual, c.expected)
		}
	}
}

func TestFormatInsertSQL(t *testing.T) {
	d := &MySQLDriver{}
	cols := []string{"id", "username", "email", "active"}
	rows := [][]any{
		{int64(1), "alice", "alice@example.com", true},
		{int64(2), "bob", nil, false},
	}

	sql, err := d.FormatInsertSQL("users", cols, rows)
	if err != nil {
		t.Fatalf("failed to format insert SQL: %v", err)
	}

	expectedHeader := "INSERT INTO `users` (`id`, `username`, `email`, `active`) VALUES"
	if !strings.Contains(sql, expectedHeader) {
		t.Errorf("expected header %q, got %q", expectedHeader, sql)
	}

	if !strings.Contains(sql, "(1, 'alice', 'alice@example.com', 1)") {
		t.Errorf("expected row 1 values, got %s", sql)
	}
	if !strings.Contains(sql, "(2, 'bob', NULL, 0)") {
		t.Errorf("expected row 2 with NULL, got %s", sql)
	}
}
