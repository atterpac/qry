package engine

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/qry/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

// MySQLProvider implements Provider for MySQL databases.
type MySQLProvider struct {
	mu        sync.RWMutex
	db        *sql.DB
	dsn       string
	connected bool
}

func (p *MySQLProvider) Connect(ctx context.Context, cfg config.ConnectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	dsn := cfg.DSN
	if dsn == "" {
		return fmt.Errorf("mysql requires a DSN")
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("opening mysql: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("pinging mysql: %w", err)
	}

	p.db = db
	p.dsn = dsn
	p.connected = true
	return nil
}

func (p *MySQLProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *MySQLProvider) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return fmt.Errorf("not connected")
	}
	return p.db.PingContext(ctx)
}

func (p *MySQLProvider) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

func (p *MySQLProvider) EngineType() config.EngineType {
	return config.EngineMySQL
}

func (p *MySQLProvider) ServerVersion(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return "", fmt.Errorf("not connected")
	}
	var version string
	err := p.db.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version)
	return "MySQL " + version, err
}

func (p *MySQLProvider) Capabilities() EngineCapabilities {
	return EngineCapabilities{
		HasSchemas:        false, // MySQL uses databases-as-schemas
		HasDatabases:      true,
		HasNamespaces:     false,
		SupportsReturning: false,
		IdentifierQuote:   "`",
	}
}

func (p *MySQLProvider) ListDatabases(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.db.QueryContext(ctx, "SHOW DATABASES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var databases []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		// Skip system databases
		if name != "information_schema" && name != "performance_schema" && name != "mysql" && name != "sys" {
			databases = append(databases, name)
		}
	}
	return databases, rows.Err()
}

func (p *MySQLProvider) ListSchemas(_ context.Context, _ string) ([]string, error) {
	// MySQL doesn't have schemas separate from databases
	return nil, nil
}

func (p *MySQLProvider) ListTables(ctx context.Context, _ string) ([]TableInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.db.QueryContext(ctx, "SHOW FULL TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var name, tableType string
		if err := rows.Scan(&name, &tableType); err != nil {
			return nil, err
		}
		t := TableInfo{Name: name}
		if tableType == "VIEW" {
			t.Type = "view"
		} else {
			t.Type = "table"
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (p *MySQLProvider) DescribeTable(ctx context.Context, _, table string) ([]ColumnInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.db.QueryContext(ctx, fmt.Sprintf("DESCRIBE %s", p.QuoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var field, colType, null, key string
		var dflt sql.NullString
		var extra string
		if err := rows.Scan(&field, &colType, &null, &key, &dflt, &extra); err != nil {
			return nil, err
		}
		col := ColumnInfo{
			Name:         field,
			DataType:     colType,
			Nullable:     null == "YES",
			IsPrimaryKey: key == "PRI",
			Extra:        extra,
		}
		if dflt.Valid {
			col.Default = dflt.String
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (p *MySQLProvider) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	start := time.Now()
	query = strings.TrimSpace(query)
	upper := strings.ToUpper(query)

	isRead := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "DESCRIBE") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")

	if isRead {
		return p.executeSelect(ctx, query, start)
	}
	return p.executeExec(ctx, query, start)
}

func (p *MySQLProvider) executeSelect(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	rows, err := p.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var resultRows [][]string
	for rows.Next() {
		values := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make([]string, len(cols))
		for i, v := range values {
			if v.Valid {
				row[i] = v.String
			} else {
				row[i] = "NULL"
			}
		}
		resultRows = append(resultRows, row)
	}

	return &QueryResult{
		Columns:  cols,
		Rows:     resultRows,
		RowCount: len(resultRows),
		Duration: duration(time.Since(start)),
	}, rows.Err()
}

func (p *MySQLProvider) executeExec(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	return p.executeExecArgs(ctx, query, start, nil)
}

func (p *MySQLProvider) executeExecArgs(ctx context.Context, query string, start time.Time, args []any) (*QueryResult, error) {
	result, err := p.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	affected, _ := result.RowsAffected()
	return &QueryResult{
		RowCount: int(affected),
		Duration: duration(time.Since(start)),
		Message:  fmt.Sprintf("%d row(s) affected", affected),
	}, nil
}

func (p *MySQLProvider) ExecuteArgs(ctx context.Context, query string, args []any) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	start := time.Now()
	return p.executeExecArgs(ctx, query, start, args)
}

func (p *MySQLProvider) BuildUpdate(_, table string, pkCols []string, changes []CellChange, pkValues map[string]string) (string, []any, error) {
	if len(changes) == 0 {
		return "", nil, fmt.Errorf("no changes")
	}
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	var setClauses []string
	var args []any

	for _, ch := range changes {
		setClauses = append(setClauses, fmt.Sprintf("%s = ?", p.QuoteIdentifier(ch.Column)))
		args = append(args, ch.NewValue)
	}

	var whereClauses []string
	for _, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = ?", p.QuoteIdentifier(pk)))
		args = append(args, pkValues[pk])
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		p.QuoteIdentifier(table),
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *MySQLProvider) BuildInsert(_, table string, columns []string, values []string) (string, []any, error) {
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("no columns")
	}

	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	args := make([]any, len(values))
	for i, col := range columns {
		quotedCols[i] = p.QuoteIdentifier(col)
		placeholders[i] = "?"
		args[i] = values[i]
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.QuoteIdentifier(table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "))

	return sql, args, nil
}

func (p *MySQLProvider) BuildDelete(_, table string, pkCols []string, pkValues map[string]string) (string, []any, error) {
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	var whereClauses []string
	var args []any
	for _, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = ?", p.QuoteIdentifier(pk)))
		args = append(args, pkValues[pk])
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		p.QuoteIdentifier(table),
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *MySQLProvider) GetForeignKeys(ctx context.Context, _, table string) ([]ForeignKeyInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Get current database name
	var dbName string
	if err := p.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&dbName); err != nil {
		return nil, err
	}

	// Outgoing FKs
	outQuery := `
		SELECT COLUMN_NAME, REFERENCED_TABLE_SCHEMA, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL`

	rows, err := p.db.QueryContext(ctx, outQuery, dbName, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []ForeignKeyInfo
	for rows.Next() {
		var fk ForeignKeyInfo
		if err := rows.Scan(&fk.Column, &fk.ReferencedSchema, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		fks = append(fks, fk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Inbound FKs
	inQuery := `
		SELECT REFERENCED_COLUMN_NAME, TABLE_SCHEMA, TABLE_NAME, COLUMN_NAME
		FROM information_schema.KEY_COLUMN_USAGE
		WHERE REFERENCED_TABLE_SCHEMA = ? AND REFERENCED_TABLE_NAME = ? AND REFERENCED_TABLE_NAME IS NOT NULL`

	rows2, err := p.db.QueryContext(ctx, inQuery, dbName, table)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()

	for rows2.Next() {
		var fk ForeignKeyInfo
		if err := rows2.Scan(&fk.Column, &fk.ReferencedSchema, &fk.ReferencedTable, &fk.ReferencedColumn); err != nil {
			return nil, err
		}
		fk.IsInbound = true
		fks = append(fks, fk)
	}
	return fks, rows2.Err()
}

func (p *MySQLProvider) BuildSearchClause(columns []string, filters []SearchFilter) string {
	if len(filters) == 0 || len(columns) == 0 {
		return ""
	}

	var andParts []string
	for _, f := range filters {
		escaped := strings.ReplaceAll(f.Value, "'", "''")
		if f.Column != "" {
			andParts = append(andParts, fmt.Sprintf("CAST(%s AS CHAR) LIKE '%%%s%%'", p.QuoteIdentifier(f.Column), escaped))
		} else {
			var orParts []string
			for _, col := range columns {
				orParts = append(orParts, fmt.Sprintf("CAST(%s AS CHAR) LIKE '%%%s%%'", p.QuoteIdentifier(col), escaped))
			}
			andParts = append(andParts, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	return strings.Join(andParts, " AND ")
}

func (p *MySQLProvider) QuoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
