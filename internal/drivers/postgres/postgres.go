package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
)

func init() {
	drivers.Register("postgres", &PostgresDriver{})
}

// PostgresDriver implements drivers.Driver for PostgreSQL 12–17+.
type PostgresDriver struct {
	db   *sql.DB
	opts config.SourceOptions
}

func (p *PostgresDriver) Name() string {
	return "postgres"
}

func (p *PostgresDriver) Open(ctx context.Context, dsn string, opts config.SourceOptions) error {
	p.opts = opts

	// Standard PostgreSQL connection (using standard database/sql interface)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	maxConns := opts.MaxOpenConns
	if maxConns <= 0 {
		maxConns = 5
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxLifetime(30 * time.Minute)

	p.db = db
	return nil
}

func (p *PostgresDriver) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *PostgresDriver) Ping(ctx context.Context) error {
	if p.db == nil {
		return fmt.Errorf("driver not initialized")
	}
	return p.db.PingContext(ctx)
}

func (p *PostgresDriver) GetTables(ctx context.Context) ([]string, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = current_schema() 
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name
	`
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query PostgreSQL tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (p *PostgresDriver) GetTableEstimatedRows(ctx context.Context, table string) (int64, error) {
	query := `
		SELECT reltuples::bigint 
		FROM pg_class c 
		JOIN pg_namespace n ON n.oid = c.relnamespace 
		WHERE n.nspname = current_schema() AND c.relname = $1
	`
	var rows sql.NullInt64
	if err := p.db.QueryRowContext(ctx, query, table).Scan(&rows); err != nil {
		return 0, nil
	}
	if rows.Valid && rows.Int64 > 0 {
		return rows.Int64, nil
	}
	return 0, nil
}

func (p *PostgresDriver) GetTableSchema(ctx context.Context, table string) (*drivers.TableSchema, error) {
	query := `
		SELECT 
			c.column_name, 
			c.data_type, 
			c.is_nullable, 
			c.is_generated,
			c.is_identity,
			c.column_default,
			COALESCE((
				SELECT 'PRI' 
				FROM information_schema.table_constraints tc 
				JOIN information_schema.constraint_column_usage ccu 
				  ON tc.constraint_name = ccu.constraint_name 
				WHERE tc.table_schema = current_schema() 
				  AND tc.table_name = c.table_name 
				  AND ccu.column_name = c.column_name 
				  AND tc.constraint_type = 'PRIMARY KEY' 
				LIMIT 1
			), '') AS pk
		FROM information_schema.columns c
		WHERE c.table_schema = current_schema() AND c.table_name = $1
		ORDER BY c.ordinal_position
	`
	rows, err := p.db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query schema for `%s`: %w", table, err)
	}
	defer rows.Close()

	schema := &drivers.TableSchema{TableName: table}

	for rows.Next() {
		var colName, dataType, isNullable, isGenerated, isIdentity, pk string
		var defaultVal sql.NullString

		if err := rows.Scan(&colName, &dataType, &isNullable, &isGenerated, &isIdentity, &defaultVal, &pk); err != nil {
			return nil, err
		}

		isAutoIncr := strings.EqualFold(isIdentity, "YES") ||
			(defaultVal.Valid && strings.Contains(strings.ToLower(defaultVal.String), "nextval("))

		col := drivers.ColumnInfo{
			Name:         colName,
			DataType:     dataType,
			IsNullable:   strings.EqualFold(isNullable, "YES"),
			IsPrimaryKey: pk == "PRI",
			IsGenerated:  strings.EqualFold(isGenerated, "ALWAYS") || strings.EqualFold(isGenerated, "NEVER") == false && isGenerated != "",
			IsAutoIncr:   isAutoIncr,
		}
		if defaultVal.Valid {
			v := defaultVal.String
			col.DefaultValue = &v
		}

		if col.IsPrimaryKey {
			schema.PKColumns = append(schema.PKColumns, colName)
		}
		schema.Columns = append(schema.Columns, col)
	}

	return schema, rows.Err()
}

func (p *PostgresDriver) GetForeignKeys(ctx context.Context) ([]drivers.ForeignKey, error) {
	query := `
		SELECT 
			tc.constraint_name, 
			tc.table_name, 
			kcu.column_name, 
			ccu.table_name AS referenced_table, 
			ccu.column_name AS referenced_column
		FROM information_schema.table_constraints AS tc
		JOIN information_schema.key_column_usage AS kcu
		  ON tc.constraint_name = kcu.constraint_name AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage AS ccu
		  ON ccu.constraint_name = tc.constraint_name AND ccu.table_schema = tc.table_schema
		WHERE tc.constraint_type = 'FOREIGN KEY' AND tc.table_schema = current_schema()
		ORDER BY tc.table_name, tc.constraint_name
	`
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query PostgreSQL foreign keys: %w", err)
	}
	defer rows.Close()

	var fks []drivers.ForeignKey
	for rows.Next() {
		var constraint, fromTable, fromCol, toTable, toCol string
		if err := rows.Scan(&constraint, &fromTable, &fromCol, &toTable, &toCol); err != nil {
			return nil, err
		}
		fks = append(fks, drivers.ForeignKey{
			ConstraintName: constraint,
			FromTable:      fromTable,
			FromColumn:     fromCol,
			ToTable:        toTable,
			ToColumn:       toCol,
		})
	}
	return fks, rows.Err()
}

func (p *PostgresDriver) QueryIDs(ctx context.Context, query string) ([]any, error) {
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query %q: %w", query, err)
	}
	defer rows.Close()

	var ids []any
	for rows.Next() {
		var id any
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (p *PostgresDriver) QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error) {
	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf(`"%s"`, c))
	}
	query := fmt.Sprintf(`SELECT %s FROM "%s"`, strings.Join(quotedCols, ", "), table)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	numCols := len(columns)
	var results [][]any
	for rows.Next() {
		scanArgs := make([]any, numCols)
		rowVals := make([]any, numCols)
		for i := range scanArgs {
			scanArgs[i] = &rowVals[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}
		results = append(results, rowVals)
	}
	return results, rows.Err()
}

func (p *PostgresDriver) StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback drivers.RowCallback) error {
	if batchSize <= 0 {
		batchSize = 2000
	}
	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf(`"%s"`, c))
	}
	query := fmt.Sprintf(`SELECT %s FROM "%s"`, strings.Join(quotedCols, ", "), table)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	// Snapshot read isolation
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to start PG snapshot read transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()

	numCols := len(columns)
	batch := make([][]any, 0, batchSize)

	for rows.Next() {
		scanArgs := make([]any, numCols)
		rowVals := make([]any, numCols)
		for i := range scanArgs {
			scanArgs[i] = &rowVals[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return err
		}

		batch = append(batch, rowVals)
		if len(batch) >= batchSize {
			if err := callback(batch); err != nil {
				return err
			}
			batch = batch[:0]
			if p.opts.ThrottleSleepMs > 0 {
				time.Sleep(time.Duration(p.opts.ThrottleSleepMs) * time.Millisecond)
			}
		}
	}

	if len(batch) > 0 {
		if err := callback(batch); err != nil {
			return err
		}
	}

	return rows.Err()
}

func (p *PostgresDriver) FormatInsertSQL(table string, columns []string, rows [][]any) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}
	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf(`"%s"`, c))
	}
	header := fmt.Sprintf(`INSERT INTO "%s" (%s) OVERRIDING SYSTEM VALUE VALUES`+"\n", table, strings.Join(quotedCols, ", "))

	var valRows []string
	for _, row := range rows {
		var valStrs []string
		for _, val := range row {
			valStrs = append(valStrs, formatPGValue(val))
		}
		valRows = append(valRows, "  ("+strings.Join(valStrs, ", ")+")")
	}

	return header + strings.Join(valRows, ",\n") + ";\n", nil
}

func (p *PostgresDriver) SessionHeaderSQL() string {
	return "-- dbseal PostgreSQL safe session initialization\n" +
		"SET client_encoding = 'UTF8';\n" +
		"SET standard_conforming_strings = on;\n" +
		"SET check_function_bodies = false;\n" +
		"SET client_min_messages = warning;\n\n"
}

func (p *PostgresDriver) SessionFooterSQL() string {
	return "\n-- dbseal PostgreSQL session complete\n"
}

func (p *PostgresDriver) ExecuteSQL(ctx context.Context, sql string) error {
	_, err := p.db.ExecContext(ctx, sql)
	return err
}

func (p *PostgresDriver) WipeTable(ctx context.Context, table string) error {
	query := fmt.Sprintf(`DELETE FROM "%s"`, table)
	_, err := p.db.ExecContext(ctx, query)
	return err
}

func (p *PostgresDriver) ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error {
	// In PostgreSQL, reset sequences associated with table
	query := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('"%s"', 'id'), %d, false)`, table, nextVal)
	_, err := p.db.ExecContext(ctx, query)
	return err
}

func (p *PostgresDriver) SetForeignKeyChecks(ctx context.Context, enabled bool) error {
	role := "origin"
	if !enabled {
		role = "replica"
	}
	query := fmt.Sprintf("SET session_replication_role = '%s'", role)
	_, err := p.db.ExecContext(ctx, query)
	return err
}

func formatPGValue(val any) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05.999999Z07:00"))
	case []byte:
		return fmt.Sprintf(`'\x%x'`, v)
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	default:
		return "'" + strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''") + "'"
	}
}
