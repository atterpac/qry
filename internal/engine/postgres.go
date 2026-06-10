package engine

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/qry/internal/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresProvider implements Provider for PostgreSQL databases.
type PostgresProvider struct {
	mu        sync.RWMutex
	pool      *pgxpool.Pool
	dsn       string
	connected bool

	// Transaction state
	tx   pgx.Tx
	conn *pgxpool.Conn // held while tx is active
}

func (p *PostgresProvider) Connect(ctx context.Context, cfg config.ConnectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	dsn := cfg.DSN
	if dsn == "" {
		return fmt.Errorf("postgres requires a DSN")
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connecting to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("pinging postgres: %w", err)
	}

	p.pool = pool
	p.dsn = dsn
	p.connected = true
	return nil
}

func (p *PostgresProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	if p.pool != nil {
		p.pool.Close()
	}
	return nil
}

func (p *PostgresProvider) Ping(ctx context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return fmt.Errorf("not connected")
	}
	return p.pool.Ping(ctx)
}

func (p *PostgresProvider) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

func (p *PostgresProvider) EngineType() config.EngineType {
	return config.EnginePostgres
}

func (p *PostgresProvider) ServerVersion(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return "", fmt.Errorf("not connected")
	}
	var version string
	err := p.pool.QueryRow(ctx, "SHOW server_version").Scan(&version)
	return "PostgreSQL " + version, err
}

func (p *PostgresProvider) Capabilities() EngineCapabilities {
	return EngineCapabilities{
		HasSchemas:        true,
		HasDatabases:      true,
		HasNamespaces:     false,
		SupportsReturning: true,
		IdentifierQuote:   `"`,
	}
}

func (p *PostgresProvider) BeginTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx != nil {
		return fmt.Errorf("transaction already active")
	}
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquiring connection: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		return fmt.Errorf("beginning transaction: %w", err)
	}
	p.conn = conn
	p.tx = tx
	return nil
}

func (p *PostgresProvider) CommitTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := p.tx.Commit(ctx)
	p.tx = nil
	p.conn.Release()
	p.conn = nil
	return err
}

func (p *PostgresProvider) RollbackTx(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tx == nil {
		return fmt.Errorf("no active transaction")
	}
	err := p.tx.Rollback(ctx)
	p.tx = nil
	p.conn.Release()
	p.conn = nil
	return err
}

func (p *PostgresProvider) InTransaction() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.tx != nil
}

func (p *PostgresProvider) ListDatabases(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.pool.Query(ctx, "SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname")
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
		databases = append(databases, name)
	}
	return databases, rows.Err()
}

func (p *PostgresProvider) ListSchemas(ctx context.Context, _ string) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	rows, err := p.pool.Query(ctx, "SELECT schema_name FROM information_schema.schemata WHERE schema_name NOT IN ('information_schema', 'pg_catalog', 'pg_toast') ORDER BY schema_name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schemas []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		schemas = append(schemas, name)
	}
	return schemas, rows.Err()
}

func (p *PostgresProvider) ListTables(ctx context.Context, schema string) ([]TableInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	if schema == "" {
		schema = "public"
	}

	rows, err := p.pool.Query(ctx,
		"SELECT table_name, table_type FROM information_schema.tables WHERE table_schema = $1 ORDER BY table_name",
		schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var t TableInfo
		var tableType string
		if err := rows.Scan(&t.Name, &tableType); err != nil {
			return nil, err
		}
		t.Schema = schema
		switch tableType {
		case "BASE TABLE":
			t.Type = "table"
		case "VIEW":
			t.Type = "view"
		default:
			t.Type = strings.ToLower(tableType)
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

func (p *PostgresProvider) DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	if schema == "" {
		schema = "public"
	}

	// Get columns with PK info
	query := `
		SELECT
			c.column_name,
			c.data_type,
			c.is_nullable,
			COALESCE(c.column_default, ''),
			CASE WHEN pk.column_name IS NOT NULL THEN true ELSE false END as is_pk
		FROM information_schema.columns c
		LEFT JOIN (
			SELECT kcu.column_name
			FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu
				ON tc.constraint_name = kcu.constraint_name
				AND tc.table_schema = kcu.table_schema
			WHERE tc.constraint_type = 'PRIMARY KEY'
				AND tc.table_schema = $1
				AND tc.table_name = $2
		) pk ON c.column_name = pk.column_name
		WHERE c.table_schema = $1 AND c.table_name = $2
		ORDER BY c.ordinal_position`

	rows, err := p.pool.Query(ctx, query, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []ColumnInfo
	for rows.Next() {
		var col ColumnInfo
		var nullable string
		var isPK bool
		if err := rows.Scan(&col.Name, &col.DataType, &nullable, &col.Default, &isPK); err != nil {
			return nil, err
		}
		col.Nullable = nullable == "YES"
		col.IsPrimaryKey = isPK
		columns = append(columns, col)
	}
	return columns, rows.Err()
}

func (p *PostgresProvider) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	start := time.Now()
	query = strings.TrimSpace(query)
	upper := strings.ToUpper(query)

	isRead := strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "WITH") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "TABLE")

	if isRead {
		return p.executeSelect(ctx, query, start)
	}
	return p.executeExec(ctx, query, start)
}

// pgQueryer abstracts pool and tx for query execution.
type pgQueryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// queryer returns the transaction if active, otherwise the pool.
// Must be called while holding at least an RLock.
func (p *PostgresProvider) queryer() pgQueryer {
	if p.tx != nil {
		return p.tx
	}
	return p.pool
}

func (p *PostgresProvider) executeSelect(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	rows, err := p.queryer().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	descs := rows.FieldDescriptions()
	cols := make([]string, len(descs))
	for i, d := range descs {
		cols[i] = d.Name
	}

	var resultRows [][]string
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := make([]string, len(values))
		for i, v := range values {
			if v == nil {
				row[i] = "NULL"
			} else {
				row[i] = formatPgValue(v)
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

func (p *PostgresProvider) executeExec(ctx context.Context, query string, start time.Time) (*QueryResult, error) {
	return p.executeExecArgs(ctx, query, start, nil)
}

func (p *PostgresProvider) executeExecArgs(ctx context.Context, query string, start time.Time, args []any) (*QueryResult, error) {
	tag, err := p.queryer().Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}

	return &QueryResult{
		RowCount: int(tag.RowsAffected()),
		Duration: duration(time.Since(start)),
		Message:  fmt.Sprintf("%d row(s) affected", tag.RowsAffected()),
	}, nil
}

func (p *PostgresProvider) ExecuteArgs(ctx context.Context, query string, args []any) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}
	start := time.Now()
	return p.executeExecArgs(ctx, query, start, args)
}

// formatPgValue converts a pgx value to a display string, handling types like
// UUIDs ([16]byte) that fmt.Sprintf("%v") renders poorly.
func formatPgValue(v any) string {
	switch val := v.(type) {
	case [16]byte:
		// UUID — format as xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
		h := hex.EncodeToString(val[:])
		return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
	case []byte:
		if len(val) == 16 {
			h := hex.EncodeToString(val)
			return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
		}
		return string(val)
	case map[string]any, []any:
		// pgx decodes JSON/JSONB into native Go types — marshal back to JSON
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (p *PostgresProvider) BuildUpdate(schema, table string, pkCols []string, changes []CellChange, pkValues map[string]string) (string, []any, error) {
	if len(changes) == 0 {
		return "", nil, fmt.Errorf("no changes")
	}
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	qualifiedTable := p.qualifiedTable(schema, table)
	var setClauses []string
	var args []any
	idx := 1

	for _, ch := range changes {
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", p.QuoteIdentifier(ch.Column), idx))
		args = append(args, ch.NewValue)
		idx++
	}

	var whereClauses []string
	for _, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", p.QuoteIdentifier(pk), idx))
		args = append(args, pkValues[pk])
		idx++
	}

	sql := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		qualifiedTable,
		strings.Join(setClauses, ", "),
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *PostgresProvider) BuildInsert(schema, table string, columns []string, values []string) (string, []any, error) {
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("no columns")
	}

	qualifiedTable := p.qualifiedTable(schema, table)
	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	args := make([]any, len(values))
	for i, col := range columns {
		quotedCols[i] = p.QuoteIdentifier(col)
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = values[i]
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		qualifiedTable,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "))

	return sql, args, nil
}

func (p *PostgresProvider) BuildDelete(schema, table string, pkCols []string, pkValues map[string]string) (string, []any, error) {
	if len(pkCols) == 0 {
		return "", nil, fmt.Errorf("no primary key columns")
	}

	qualifiedTable := p.qualifiedTable(schema, table)
	var whereClauses []string
	var args []any
	for i, pk := range pkCols {
		whereClauses = append(whereClauses, fmt.Sprintf("%s = $%d", p.QuoteIdentifier(pk), i+1))
		args = append(args, pkValues[pk])
	}

	sql := fmt.Sprintf("DELETE FROM %s WHERE %s",
		qualifiedTable,
		strings.Join(whereClauses, " AND "))

	return sql, args, nil
}

func (p *PostgresProvider) QuoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (p *PostgresProvider) BuildSearchClause(columns []string, filters []SearchFilter) string {
	if len(filters) == 0 || len(columns) == 0 {
		return ""
	}

	var andParts []string
	for _, f := range filters {
		escaped := strings.ReplaceAll(f.Value, "'", "''")
		if f.Column != "" {
			andParts = append(andParts, fmt.Sprintf("CAST(%s AS TEXT) ILIKE '%%%s%%'", p.QuoteIdentifier(f.Column), escaped))
		} else {
			var orParts []string
			for _, col := range columns {
				orParts = append(orParts, fmt.Sprintf("CAST(%s AS TEXT) ILIKE '%%%s%%'", p.QuoteIdentifier(col), escaped))
			}
			andParts = append(andParts, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	return strings.Join(andParts, " AND ")
}

func (p *PostgresProvider) GetForeignKeys(ctx context.Context, schema, table string) ([]ForeignKeyInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.pool == nil {
		return nil, fmt.Errorf("not connected")
	}

	if schema == "" {
		schema = "public"
	}

	// Outgoing FKs: this table references other tables
	outQuery := `
		SELECT
			kcu.column_name,
			ccu.table_schema,
			ccu.table_name,
			ccu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND tc.table_schema = $1
			AND tc.table_name = $2`

	rows, err := p.pool.Query(ctx, outQuery, schema, table)
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

	// Inbound FKs: other tables reference this table
	inQuery := `
		SELECT
			ccu.column_name,
			kcu.table_schema,
			kcu.table_name,
			kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
			ON tc.constraint_name = kcu.constraint_name
			AND tc.table_schema = kcu.table_schema
		JOIN information_schema.constraint_column_usage ccu
			ON tc.constraint_name = ccu.constraint_name
			AND tc.table_schema = ccu.constraint_schema
		WHERE tc.constraint_type = 'FOREIGN KEY'
			AND ccu.table_schema = $1
			AND ccu.table_name = $2`

	rows2, err := p.pool.Query(ctx, inQuery, schema, table)
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

func (p *PostgresProvider) qualifiedTable(schema, table string) string {
	if schema != "" && schema != "public" {
		return p.QuoteIdentifier(schema) + "." + p.QuoteIdentifier(table)
	}
	return p.QuoteIdentifier(table)
}

// ExplainPlan implements Provider.
func (p *PostgresProvider) ExplainPlan(ctx context.Context, sql string) (*PlanResult, error) {
	return explainPostgres(ctx, p, strings.TrimSpace(sql))
}
