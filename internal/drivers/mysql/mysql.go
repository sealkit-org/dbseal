package mysql

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sealkit/dbseal/internal/config"
	"github.com/sealkit/dbseal/internal/drivers"
)

func init() {
	drivers.Register("mysql", &MySQLDriver{})
}

// MySQLDriver implements drivers.Driver for MySQL 5.7+ / 8.0+.
type MySQLDriver struct {
	db   *sql.DB
	opts config.SourceOptions
}

func (m *MySQLDriver) Name() string {
	return "mysql"
}

func (m *MySQLDriver) Open(ctx context.Context, dsn string, opts config.SourceOptions) error {
	m.opts = opts
	// Ensure multiStatements & parseTime are handled cleanly
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	maxConns := opts.MaxOpenConns
	if maxConns <= 0 {
		maxConns = 5
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping MySQL server: %w", err)
	}

	m.db = db
	return nil
}

func (m *MySQLDriver) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *MySQLDriver) Ping(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("driver not initialized")
	}
	return m.db.PingContext(ctx)
}

func (m *MySQLDriver) GetTables(ctx context.Context) ([]string, error) {
	query := "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'"
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var table, tableType string
		if err := rows.Scan(&table, &tableType); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	return tables, rows.Err()
}

func (m *MySQLDriver) GetTableEstimatedRows(ctx context.Context, table string) (int64, error) {
	query := `SELECT TABLE_ROWS FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`
	var rows sql.NullInt64
	if err := m.db.QueryRowContext(ctx, query, table).Scan(&rows); err != nil {
		return 0, err
	}
	if rows.Valid {
		return rows.Int64, nil
	}
	return 0, nil
}

func (m *MySQLDriver) GetTableSchema(ctx context.Context, table string) (*drivers.TableSchema, error) {
	query := `
		SELECT 
			COLUMN_NAME, DATA_TYPE, IS_NULLABLE, COLUMN_KEY, EXTRA, COLUMN_DEFAULT
		FROM information_schema.COLUMNS 
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY ORDINAL_POSITION
	`
	rows, err := m.db.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("failed to query column schema for %q: %w", table, err)
	}
	defer rows.Close()

	schema := &drivers.TableSchema{TableName: table}

	for rows.Next() {
		var colName, dataType, isNullable, colKey, extra string
		var defaultVal sql.NullString

		if err := rows.Scan(&colName, &dataType, &isNullable, &colKey, &extra, &defaultVal); err != nil {
			return nil, err
		}

		extraUpper := strings.ToUpper(extra)
		isGenerated := strings.Contains(extraUpper, "VIRTUAL GENERATED") || strings.Contains(extraUpper, "STORED GENERATED")
		isAutoIncr := strings.Contains(extraUpper, "AUTO_INCREMENT")

		col := drivers.ColumnInfo{
			Name:         colName,
			DataType:     dataType,
			IsNullable:   strings.EqualFold(isNullable, "YES"),
			IsPrimaryKey: colKey == "PRI",
			IsGenerated:  isGenerated,
			IsAutoIncr:   isAutoIncr,
			Extra:        extra,
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

func (m *MySQLDriver) GetForeignKeys(ctx context.Context) ([]drivers.ForeignKey, error) {
	query := `
		SELECT 
			CONSTRAINT_NAME, 
			TABLE_NAME, 
			COLUMN_NAME, 
			REFERENCED_TABLE_NAME, 
			REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() 
		  AND REFERENCED_TABLE_NAME IS NOT NULL
		ORDER BY TABLE_NAME, CONSTRAINT_NAME, ORDINAL_POSITION
	`
	rows, err := m.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query foreign keys: %w", err)
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

func (m *MySQLDriver) QueryIDs(ctx context.Context, query string) ([]any, error) {
	rows, err := m.db.QueryContext(ctx, query)
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
		ids = append(ids, normalizeScanValue(id))
	}
	return ids, rows.Err()
}

func (m *MySQLDriver) QueryRowsWhere(ctx context.Context, table string, columns []string, whereClause string, args []any) ([][]any, error) {
	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf("`%s`", c))
	}
	colList := strings.Join(quotedCols, ", ")

	query := fmt.Sprintf("SELECT %s FROM `%s`", colList, table)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query rows from %q: %w", table, err)
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

		processedRow := make([]any, numCols)
		for i, val := range rowVals {
			processedRow[i] = normalizeScanValue(val)
		}
		results = append(results, processedRow)
	}

	return results, rows.Err()
}

func (m *MySQLDriver) StreamRows(ctx context.Context, table string, columns []string, whereClause string, batchSize int, callback drivers.RowCallback) error {
	if batchSize <= 0 {
		batchSize = 2000
	}

	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf("`%s`", c))
	}
	colList := strings.Join(quotedCols, ", ")

	query := fmt.Sprintf("SELECT %s FROM `%s`", colList, table)
	if strings.TrimSpace(whereClause) != "" {
		query += " WHERE " + whereClause
	}

	// Read snapshot isolation with Repeatable Read, no locks
	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return fmt.Errorf("failed to start snapshot read transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to execute streaming query on %q: %w", table, err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return err
	}
	numCols := len(colTypes)

	batch := make([][]any, 0, batchSize)

	for rows.Next() {
		// Scan into interface pointers
		scanArgs := make([]any, numCols)
		rowVals := make([]any, numCols)
		for i := range scanArgs {
			scanArgs[i] = &rowVals[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}

		// Convert bytes if driver returned []byte for strings/blobs
		processedRow := make([]any, numCols)
		for i, val := range rowVals {
			processedRow[i] = normalizeScanValue(val)
		}

		batch = append(batch, processedRow)

		if len(batch) >= batchSize {
			if err := callback(batch); err != nil {
				return err
			}
			batch = batch[:0]

			// Throttle sleep if configured to protect production DB
			if m.opts.ThrottleSleepMs > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(m.opts.ThrottleSleepMs) * time.Millisecond):
				}
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

func (m *MySQLDriver) FormatInsertSQL(table string, columns []string, rows [][]any) (string, error) {
	if len(rows) == 0 {
		return "", nil
	}

	var quotedCols []string
	for _, c := range columns {
		quotedCols = append(quotedCols, fmt.Sprintf("`%s`", c))
	}
	header := fmt.Sprintf("INSERT INTO `%s` (%s) VALUES\n", table, strings.Join(quotedCols, ", "))

	var valRows []string
	for _, row := range rows {
		var valStrs []string
		for _, val := range row {
			valStrs = append(valStrs, formatMySQLValue(val))
		}
		valRows = append(valRows, "  ("+strings.Join(valStrs, ", ")+")")
	}

	return header + strings.Join(valRows, ",\n") + ";\n", nil
}

func (m *MySQLDriver) SessionHeaderSQL() string {
	return "-- dbseal generated dump: safe session initialization\n" +
		"/*!40101 SET @OLD_CHARACTER_SET_CLIENT=@@CHARACTER_SET_CLIENT */;\n" +
		"/*!40101 SET NAMES utf8mb4 */;\n" +
		"/*!40014 SET @OLD_FOREIGN_KEY_CHECKS=@@FOREIGN_KEY_CHECKS, FOREIGN_KEY_CHECKS=0 */;\n" +
		"/*!40101 SET @OLD_SQL_MODE=@@SQL_MODE, SQL_MODE='NO_AUTO_VALUE_ON_ZERO' */;\n" +
		"/*!40111 SET @OLD_SQL_NOTES=@@SQL_NOTES, SQL_NOTES=0 */;\n\n"
}

func (m *MySQLDriver) SessionFooterSQL() string {
	return "\n-- dbseal session restoration\n" +
		"/*!40014 SET FOREIGN_KEY_CHECKS=@OLD_FOREIGN_KEY_CHECKS */;\n" +
		"/*!40101 SET CHARACTER_SET_CLIENT=@OLD_CHARACTER_SET_CLIENT */;\n"
}

func (m *MySQLDriver) ExecuteSQL(ctx context.Context, sql string) error {
	_, err := m.db.ExecContext(ctx, sql)
	return err
}

func (m *MySQLDriver) WipeTable(ctx context.Context, table string) error {
	// Use DELETE FROM rather than TRUNCATE to avoid InnoDB foreign key constraint errors
	query := fmt.Sprintf("DELETE FROM `%s`", table)
	_, err := m.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to wipe table `%s`: %w", table, err)
	}
	return nil
}

func (m *MySQLDriver) ResetAutoIncrement(ctx context.Context, table string, nextVal int64) error {
	if nextVal <= 0 {
		nextVal = 1
	}
	query := fmt.Sprintf("ALTER TABLE `%s` AUTO_INCREMENT = %d", table, nextVal)
	_, err := m.db.ExecContext(ctx, query)
	return err
}

func (m *MySQLDriver) SetForeignKeyChecks(ctx context.Context, enabled bool) error {
	val := 0
	if enabled {
		val = 1
	}
	query := fmt.Sprintf("SET FOREIGN_KEY_CHECKS = %d", val)
	_, err := m.db.ExecContext(ctx, query)
	return err
}

func normalizeScanValue(val any) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []byte:
		// Convert to string unless it's binary data
		return string(v)
	default:
		return v
	}
}

func formatMySQLValue(val any) string {
	if val == nil {
		return "NULL"
	}
	switch v := val.(type) {
	case bool:
		if v {
			return "1"
		}
		return "0"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%v", v)
	case time.Time:
		return fmt.Sprintf("'%s'", v.Format("2006-01-02 15:04:05.999999"))
	case []byte:
		return fmt.Sprintf("0x%s", hex.EncodeToString(v))
	case string:
		return "'" + escapeMySQLString(v) + "'"
	default:
		return "'" + escapeMySQLString(fmt.Sprintf("%v", v)) + "'"
	}
}

func escapeMySQLString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\032':
			b.WriteString(`\Z`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
