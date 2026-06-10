package engine

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/qry/internal/config"
	_ "modernc.org/sqlite"
)

// SQLiteProvider implements Provider for SQLite databases.
type SQLiteProvider struct {
	mu        sync.RWMutex
	db        *sql.DB
	path      string
	connected bool
	tx        *sql.Tx
}

func (p *SQLiteProvider) Connect(ctx context.Context, cfg config.ConnectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	path := cfg.Path
	if path == "" {
		path = cfg.DSN
	}

	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			path = filepath.Join(home, path[2:])
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("opening sqlite: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("pinging sqlite: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return fmt.Errorf("enabling foreign keys: %w", err)
	}

	p.db = db
	p.path = path
	p.connected = true
	return nil
}

func (p *SQLiteProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

func (p *SQLiteProvider) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return fmt.Errorf("not connected")
	}
	return p.db.PingContext(ctx)
}

func (p *SQLiteProvider) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

func (p *SQLiteProvider) EngineType() config.EngineType {
	return config.EngineSQLite
}

func (p *SQLiteProvider) ServerVersion(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return "", fmt.Errorf("not connected")
	}
	var version string
	err := p.db.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&version)
	return "SQLite " + version, err
}

func (p *SQLiteProvider) Capabilities() EngineCapabilities {
	return EngineCapabilities{
		HasSchemas:        false,
		HasDatabases:      false,
		HasNamespaces:     false,
		SupportsReturning: true,
		IdentifierQuote:   `"`,
	}
}

func (p *SQLiteProvider) ListDatabases(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("sqlite does not support multiple databases")
}

func (p *SQLiteProvider) ListSchemas(_ context.Context, _ string) ([]string, error) {
	return []string{"main"}, nil
}

func (p *SQLiteProvider) ListTables(ctx context.Context, _ string) ([]TableInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.db.QueryContext(ctx, "SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') AND name NOT LIKE 'sqlite_%' ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		if err := rows.Scan(&t.Name, &t.Type); err != nil {
			return nil, err
		}
		t.Schema = "main"
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (p *SQLiteProvider) DescribeTable(ctx context.Context, _, table string) ([]ColumnInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", p.QuoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		col := ColumnInfo{
			Name:         name,
			DataType:     dataType,
			Nullable:     notNull == 0,
			IsPrimaryKey: pk > 0,
		}
		if dflt.Valid {
			col.Default = dflt.String
		}
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (p *SQLiteProvider) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	start := time.Now()
	query = strings.TrimSpace(query)
	upper := strings.ToUpper(query)

	// Check if it's a SELECT or similar read query
	isRead := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH")

	if isRead {
		return p.executeSelect(ctx, query, start)
	}
	return p.executeExec(ctx, query, start)
}

// sqlDB returns the transaction if active, otherwise the db.
// Must be called while holding at least an RLock.
func (p *SQLiteProvider) sqlDB() sqlQueryer {
	if p.tx != nil {
		return p.tx
	}
	return p.db
}

func (p *SQLiteProvider) BeginTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx != nil {
		return fmt.Errorf("transaction already active")
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	p.tx = tx
	return nil
}

func (p *SQLiteProvider) CommitTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := p.tx.Commit()
	p.tx = nil
	return err
}

func (p *SQLiteProvider) RollbackTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := p.tx.Rollback()
	p.tx = nil
	return err
}

func (p *SQLiteProvider) InTransaction() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tx != nil
}

func (p *SQLiteProvider) executeSelect(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	rows, err := p.sqlDB().QueryContext(ctx, query)
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

func (p *SQLiteProvider) executeExec(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	return p.executeExecArgs(ctx, query, start, nil)
}

func (p *SQLiteProvider) executeExecArgs(ctx context.Context, query string, start time.Time, args []any) (*QueryResult, error) {
	result, err := p.sqlDB().ExecContext(ctx, query, args...)
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

func (p *SQLiteProvider) ExecuteArgs(ctx context.Context, query string, args []any) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	start := time.Now()
	return p.executeExecArgs(ctx, query, start, args)
}

func (p *SQLiteProvider) BuildUpdate(_, table string, pkCols []string, changes []CellChange, pkValues map[string]string) (string, []any, error) {
	if len(changes) == 0 {
		return "", nil, fmt.Errorf("no changes")
	}
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	var setClauses []string
	var args []any
	idx := 1

	for _, ch := range changes {
		setClauses = append(setClauses, fmt.Sprintf("%s = ?%d", p.QuoteIdentifier(ch.Column), idx))
		args = append(args, ch.NewValue)
		idx++
	}

	var whereClauses []string
	for _, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = ?%d", p.QuoteIdentifier(pk), idx))
		args = append(args, pkValues[pk])
		idx++
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		p.QuoteIdentifier(table),
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *SQLiteProvider) BuildInsert(_, table string, columns []string, values []string) (string, []any, error) {
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("no columns")
	}

	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	args := make([]any, len(values))
	for i, col := range columns {
		quotedCols[i] = p.QuoteIdentifier(col)
		placeholders[i] = fmt.Sprintf("?%d", i+1)
		args[i] = values[i]
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		p.QuoteIdentifier(table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "))

	return sql, args, nil
}

func (p *SQLiteProvider) BuildDelete(_, table string, pkCols []string, pkValues map[string]string) (string, []any, error) {
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	var whereClauses []string
	var args []any
	for i, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = ?%d", p.QuoteIdentifier(pk), i+1))
		args = append(args, pkValues[pk])
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		p.QuoteIdentifier(table),
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *SQLiteProvider) GetForeignKeys(ctx context.Context, _, table string) ([]ForeignKeyInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Outgoing FKs via PRAGMA foreign_key_list
	rows, err := p.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", p.QuoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fks []ForeignKeyInfo
	for rows.Next() {
		var id, seq int
		var refTable, from, to string
		var onUpdate, onDelete, match string
		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, err
		}
		fks = append(fks, ForeignKeyInfo{
			Column:           from,
			ReferencedTable:  refTable,
			ReferencedColumn: to,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Inbound FKs: check all other tables for references to this table
	tableRows, err := p.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name != ?", table)
	if err != nil {
		return nil, err
	}
	defer tableRows.Close()

	var otherTables []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			return nil, err
		}
		otherTables = append(otherTables, name)
	}
	if err := tableRows.Err(); err != nil {
		return nil, err
	}

	for _, otherTable := range otherTables {
		fkRows, err := p.db.QueryContext(ctx, fmt.Sprintf("PRAGMA foreign_key_list(%s)", p.QuoteIdentifier(otherTable)))
		if err != nil {
			continue
		}

		for fkRows.Next() {
			var id, seq int
			var refTable, from, to string
			var onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				fkRows.Close()
				continue
			}
			if refTable == table {
				fks = append(fks, ForeignKeyInfo{
					Column:           to,
					ReferencedTable:  otherTable,
					ReferencedColumn: from,
					IsInbound:        true,
				})
			}
		}
		fkRows.Close()
	}

	return fks, nil
}

func (p *SQLiteProvider) BuildSearchClause(columns []string, filters []SearchFilter) string {
	if len(filters) == 0 || len(columns) == 0 {
		return ""
	}

	var andParts []string
	for _, f := range filters {
		escaped := strings.ReplaceAll(f.Value, "'", "''")
		if f.Column != "" {
			andParts = append(andParts, fmt.Sprintf("CAST(%s AS TEXT) LIKE '%%%s%%'", p.QuoteIdentifier(f.Column), escaped))
		} else {
			var orParts []string
			for _, col := range columns {
				orParts = append(orParts, fmt.Sprintf("CAST(%s AS TEXT) LIKE '%%%s%%'", p.QuoteIdentifier(col), escaped))
			}
			andParts = append(andParts, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	return strings.Join(andParts, " AND ")
}

func (p *SQLiteProvider) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// duration wraps time.Duration for fmt.Stringer.
type duration time.Duration

func (d duration) String() string {
	return time.Duration(d).String()
}

// ExplainPlan implements Provider.
func (p *SQLiteProvider) ExplainPlan(ctx context.Context, sql string) (*PlanResult, error) {
	return explainSQLite(ctx, p, strings.TrimSpace(sql))
}
