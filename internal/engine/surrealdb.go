package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/qry/internal/config"
	surrealdb "github.com/surrealdb/surrealdb.go"
)

// SurrealDBProvider implements Provider for SurrealDB.
type SurrealDBProvider struct {
	mu        sync.RWMutex
	db        *surrealdb.DB
	namespace string
	database  string
	connected bool
}

func (p *SurrealDBProvider) Connect(ctx context.Context, cfg config.ConnectionConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	url := cfg.URL
	if url == "" {
		return fmt.Errorf("surrealdb requires a URL")
	}

	db, err := surrealdb.New(url)
	if err != nil {
		return fmt.Errorf("connecting to surrealdb: %w", err)
	}

	// Authenticate
	if cfg.Auth.Username != "" {
		_, err = db.SignIn(ctx, &surrealdb.Auth{
			Username: cfg.Auth.Username,
			Password: cfg.Auth.Password,
		})
		if err != nil {
			return fmt.Errorf("surrealdb auth: %w", err)
		}
	}

	// Use namespace and database
	ns := cfg.Namespace
	if ns == "" {
		ns = "default"
	}
	dbName := cfg.Database
	if dbName == "" {
		dbName = "default"
	}

	if err := db.Use(ctx, ns, dbName); err != nil {
		return fmt.Errorf("surrealdb use: %w", err)
	}

	p.db = db
	p.namespace = ns
	p.database = dbName
	p.connected = true
	return nil
}

func (p *SurrealDBProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.connected = false
	if p.db != nil {
		p.db.Close(context.Background())
	}
	return nil
}

func (p *SurrealDBProvider) Ping(_ context.Context) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return fmt.Errorf("not connected")
	}
	return nil
}

func (p *SurrealDBProvider) IsConnected() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.connected
}

func (p *SurrealDBProvider) EngineType() config.EngineType {
	return config.EngineSurrealDB
}

func (p *SurrealDBProvider) ServerVersion(ctx context.Context) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return "", fmt.Errorf("not connected")
	}
	ver, err := p.db.Version(ctx)
	if err != nil {
		return "SurrealDB (unknown)", nil
	}
	return "SurrealDB " + ver.Version, nil
}

func (p *SurrealDBProvider) Capabilities() EngineCapabilities {
	return EngineCapabilities{
		HasSchemas:        false,
		HasDatabases:      true,
		HasNamespaces:     true,
		HasRecordLinks:    true,
		HasGraphQueries:   true,
		SupportsReturning: true,
		IdentifierQuote:   "`",
	}
}

func (p *SurrealDBProvider) ListDatabases(ctx context.Context) ([]string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := surrealdb.Query[any](ctx, p.db, "INFO FOR NS", nil)
	if err != nil {
		return nil, err
	}

	return p.extractNamesFromQueryResult(result, "databases"), nil
}

func (p *SurrealDBProvider) ListSchemas(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

func (p *SurrealDBProvider) ListTables(ctx context.Context, _ string) ([]TableInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := surrealdb.Query[any](ctx, p.db, "INFO FOR DB", nil)
	if err != nil {
		return nil, err
	}

	names := p.extractNamesFromQueryResult(result, "tables")
	var tables []TableInfo
	for _, name := range names {
		tables = append(tables, TableInfo{
			Name: name,
			Type: "table",
		})
	}
	return tables, nil
}

func (p *SurrealDBProvider) DescribeTable(ctx context.Context, _, table string) ([]ColumnInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	result, err := surrealdb.Query[any](ctx, p.db, fmt.Sprintf("INFO FOR TABLE %s", p.QuoteIdentifier(table)), nil)
	if err != nil {
		return nil, err
	}

	columns := p.extractFieldsFromQueryResult(result)

	// SurrealDB tables always have an implicit "id" field
	hasID := false
	for _, col := range columns {
		if col.Name == "id" {
			hasID = true
			break
		}
	}
	if !hasID {
		columns = append([]ColumnInfo{{
			Name:         "id",
			DataType:     "record",
			IsPrimaryKey: true,
			Nullable:     false,
		}}, columns...)
	}

	return columns, nil
}

func (p *SurrealDBProvider) ExecuteArgs(ctx context.Context, query string, _ []any) (*QueryResult, error) {
	// SurrealDB inlines values in BuildUpdate/BuildInsert/BuildDelete, so args are unused.
	return p.ExecuteQuery(ctx, query)
}

func (p *SurrealDBProvider) ExecuteQuery(ctx context.Context, query string) (*QueryResult, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.db == nil {
		return nil, fmt.Errorf("not connected")
	}

	start := time.Now()

	result, err := surrealdb.Query[any](ctx, p.db, query, nil)
	if err != nil {
		return nil, err
	}

	elapsed := time.Since(start)

	if result == nil || len(*result) == 0 {
		return &QueryResult{
			Duration: duration(elapsed),
			Message:  "Query executed (no results)",
		}, nil
	}

	// Take the last query result
	lastResult := (*result)[len(*result)-1]
	if lastResult.Error != nil {
		return nil, fmt.Errorf("%v", lastResult.Error)
	}

	return p.parseResult(lastResult.Result, elapsed)
}

func (p *SurrealDBProvider) parseResult(result any, elapsed time.Duration) (*QueryResult, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return &QueryResult{
			Duration: duration(elapsed),
			Message:  "Query executed",
		}, nil
	}

	// Try to parse as array of objects
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil || len(rows) == 0 {
		return &QueryResult{
			Duration: duration(elapsed),
			Message:  "Query executed",
		}, nil
	}

	// Extract column names from first row
	var columns []string
	for key := range rows[0] {
		columns = append(columns, key)
	}

	// Build rows
	var resultRows [][]string
	for _, row := range rows {
		r := make([]string, len(columns))
		for i, col := range columns {
			if v, ok := row[col]; ok {
				if v == nil {
					r[i] = "NULL"
				} else {
					r[i] = fmt.Sprintf("%v", v)
				}
			}
		}
		resultRows = append(resultRows, r)
	}

	return &QueryResult{
		Columns:  columns,
		Rows:     resultRows,
		RowCount: len(resultRows),
		Duration: duration(elapsed),
	}, nil
}

func (p *SurrealDBProvider) extractNamesFromQueryResult(result *[]surrealdb.QueryResult[any], key string) []string {
	if result == nil || len(*result) == 0 {
		return nil
	}

	for _, qr := range *result {
		data, err := json.Marshal(qr.Result)
		if err != nil {
			continue
		}

		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}

		if items, ok := parsed[key]; ok {
			if m, ok := items.(map[string]any); ok {
				var names []string
				for name := range m {
					names = append(names, name)
				}
				return names
			}
		}
	}

	return nil
}

func (p *SurrealDBProvider) extractFieldsFromQueryResult(result *[]surrealdb.QueryResult[any]) []ColumnInfo {
	if result == nil || len(*result) == 0 {
		return nil
	}

	for _, qr := range *result {
		data, err := json.Marshal(qr.Result)
		if err != nil {
			continue
		}

		var parsed map[string]any
		if err := json.Unmarshal(data, &parsed); err != nil {
			continue
		}

		if fields, ok := parsed["fields"]; ok {
			if fm, ok := fields.(map[string]any); ok {
				var columns []ColumnInfo
				for name := range fm {
					columns = append(columns, ColumnInfo{
						Name:     name,
						DataType: "any",
						Nullable: true,
					})
				}
				return columns
			}
		}
	}

	return nil
}

func (p *SurrealDBProvider) BuildUpdate(_, table string, pkCols []string, changes []CellChange, pkValues map[string]string) (string, []any, error) {
	if len(changes) == 0 {
		return "", nil, fmt.Errorf("no changes")
	}

	// SurrealDB uses record IDs: table:id
	recordID := ""
	if id, ok := pkValues["id"]; ok {
		recordID = id
		if !strings.Contains(recordID, ":") {
			recordID = table + ":" + recordID
		}
	}

	if recordID == "" {
		return "", nil, fmt.Errorf("surrealdb requires an id for updates")
	}

	var setClauses []string
	for _, ch := range changes {
		setClauses = append(setClauses, fmt.Sprintf("%s = '%s'", p.QuoteIdentifier(ch.Column), strings.ReplaceAll(ch.NewValue, "'", "\\'")))
	}

	sql := fmt.Sprintf("UPDATE %s SET %s", recordID, strings.Join(setClauses, ", "))
	return sql, nil, nil
}

func (p *SurrealDBProvider) BuildInsert(_, table string, columns []string, values []string) (string, []any, error) {
	if len(columns) == 0 {
		return "", nil, fmt.Errorf("no columns")
	}

	var setClauses []string
	for i, col := range columns {
		setClauses = append(setClauses, fmt.Sprintf("%s: '%s'", p.QuoteIdentifier(col), strings.ReplaceAll(values[i], "'", "\\'")))
	}

	sql := fmt.Sprintf("CREATE %s CONTENT { %s }", p.QuoteIdentifier(table), strings.Join(setClauses, ", "))
	return sql, nil, nil
}

func (p *SurrealDBProvider) BuildDelete(_, table string, pkCols []string, pkValues map[string]string) (string, []any, error) {
	recordID := ""
	if id, ok := pkValues["id"]; ok {
		recordID = id
		if !strings.Contains(recordID, ":") {
			recordID = table + ":" + recordID
		}
	}

	if recordID == "" {
		return "", nil, fmt.Errorf("surrealdb requires an id for deletes")
	}

	return fmt.Sprintf("DELETE %s", recordID), nil, nil
}

func (p *SurrealDBProvider) GetForeignKeys(_ context.Context, _, _ string) ([]ForeignKeyInfo, error) {
	return nil, nil
}

func (p *SurrealDBProvider) BuildSearchClause(columns []string, filters []SearchFilter) string {
	if len(filters) == 0 || len(columns) == 0 {
		return ""
	}

	var andParts []string
	for _, f := range filters {
		escaped := strings.ReplaceAll(f.Value, "'", "\\'")
		lower := strings.ToLower(escaped)
		if f.Column != "" {
			andParts = append(andParts, fmt.Sprintf("string::lowercase(string(%s)) CONTAINS '%s'", p.QuoteIdentifier(f.Column), lower))
		} else {
			var orParts []string
			for _, col := range columns {
				orParts = append(orParts, fmt.Sprintf("string::lowercase(string(%s)) CONTAINS '%s'", p.QuoteIdentifier(col), lower))
			}
			andParts = append(andParts, "("+strings.Join(orParts, " OR ")+")")
		}
	}
	return strings.Join(andParts, " AND ")
}

func (p *SurrealDBProvider) QuoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// ExplainPlan implements Provider. SurrealDB has no SQL EXPLAIN; fall back to
// the generic EXPLAIN passthrough.
func (p *SurrealDBProvider) ExplainPlan(ctx context.Context, sql string) (*PlanResult, error) {
	return explainFallback(ctx, p, strings.TrimSpace(sql))
}
