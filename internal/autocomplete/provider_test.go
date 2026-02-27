package autocomplete

import (
	"context"
	"testing"
	"time"

	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
)

// mockProvider implements engine.Provider for testing.
type mockProvider struct {
	schemas []string
	tables  map[string][]engine.TableInfo
	columns map[string][]engine.ColumnInfo
}

func (m *mockProvider) Connect(ctx context.Context, cfg config.ConnectionConfig) error { return nil }
func (m *mockProvider) Close() error                                                   { return nil }
func (m *mockProvider) Ping(ctx context.Context) error                                 { return nil }
func (m *mockProvider) IsConnected() bool                                              { return true }
func (m *mockProvider) EngineType() config.EngineType                                  { return "postgres" }
func (m *mockProvider) ServerVersion(ctx context.Context) (string, error)              { return "15", nil }
func (m *mockProvider) Capabilities() engine.EngineCapabilities {
	return engine.EngineCapabilities{HasSchemas: true}
}
func (m *mockProvider) ListDatabases(ctx context.Context) ([]string, error) { return nil, nil }
func (m *mockProvider) ListSchemas(ctx context.Context, database string) ([]string, error) {
	return m.schemas, nil
}
func (m *mockProvider) ListTables(ctx context.Context, schema string) ([]engine.TableInfo, error) {
	return m.tables[schema], nil
}
func (m *mockProvider) DescribeTable(ctx context.Context, schema, table string) ([]engine.ColumnInfo, error) {
	return m.columns[schema+"."+table], nil
}
func (m *mockProvider) ExecuteQuery(ctx context.Context, query string) (*engine.QueryResult, error) {
	return nil, nil
}
func (m *mockProvider) ExecuteArgs(ctx context.Context, query string, args []any) (*engine.QueryResult, error) {
	return nil, nil
}
func (m *mockProvider) BuildSearchClause(columns []string, filters []engine.SearchFilter) string {
	return ""
}
func (m *mockProvider) BuildUpdate(schema, table string, pkCols []string, changes []engine.CellChange, pkValues map[string]string) (string, []any, error) {
	return "", nil, nil
}
func (m *mockProvider) BuildInsert(schema, table string, columns []string, values []string) (string, []any, error) {
	return "", nil, nil
}
func (m *mockProvider) BuildDelete(schema, table string, pkCols []string, pkValues map[string]string) (string, []any, error) {
	return "", nil, nil
}
func (m *mockProvider) QuoteIdentifier(name string) string { return `"` + name + `"` }
func (m *mockProvider) GetForeignKeys(ctx context.Context, schema, table string) ([]engine.ForeignKeyInfo, error) {
	return nil, nil
}

func newTestSetup() (*SuggestionEngine, *mockProvider) {
	provider := &mockProvider{
		schemas: []string{"public", "auth"},
		tables: map[string][]engine.TableInfo{
			"public": {
				{Schema: "public", Name: "users", Type: "table"},
				{Schema: "public", Name: "orders", Type: "table"},
				{Schema: "public", Name: "products", Type: "table"},
			},
			"auth": {
				{Schema: "auth", Name: "sessions", Type: "table"},
			},
		},
		columns: map[string][]engine.ColumnInfo{
			"public.users": {
				{Name: "id", DataType: "integer", IsPrimaryKey: true},
				{Name: "name", DataType: "text"},
				{Name: "email", DataType: "text"},
				{Name: "created_at", DataType: "timestamptz"},
			},
			"public.orders": {
				{Name: "id", DataType: "integer", IsPrimaryKey: true},
				{Name: "user_id", DataType: "integer"},
				{Name: "total", DataType: "numeric"},
			},
		},
	}

	cache := NewSchemaCache(5 * time.Minute)
	se := NewSuggestionEngine(cache, "public")
	return se, provider
}

func TestSuggest_TopLevel(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	suggestions := se.Suggest(ctx, provider, "", 0)
	// Top-level should return no suggestions (no keyword autocomplete)
	if len(suggestions) != 0 {
		t.Errorf("expected no top-level suggestions, got %d", len(suggestions))
	}
}

func TestSuggest_FromTables(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	suggestions := se.Suggest(ctx, provider, "SELECT * FROM ", len("SELECT * FROM "))

	tableFound := false
	for _, s := range suggestions {
		if s.Text == "users" && s.Category == "Table" {
			tableFound = true
		}
	}
	if !tableFound {
		t.Error("expected table 'users' in FROM suggestions")
	}
}

func TestSuggest_WhereColumns(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	sql := "SELECT * FROM users WHERE "
	suggestions := se.Suggest(ctx, provider, sql, len(sql))

	colFound := false
	for _, s := range suggestions {
		if s.Text == "name" && s.Category == "Column" {
			colFound = true
		}
	}
	if !colFound {
		t.Error("expected column 'name' in WHERE suggestions")
	}
}

func TestSuggest_AliasDot(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	sql := "SELECT * FROM users u WHERE u."
	suggestions := se.Suggest(ctx, provider, sql, len(sql))

	if len(suggestions) == 0 {
		t.Fatal("expected column suggestions after alias dot")
	}
	found := false
	for _, s := range suggestions {
		if s.Text == "email" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'email' column after u.")
	}
}

func TestSuggest_PartialFilter(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	sql := "SELECT * FROM users WHERE na"
	suggestions := se.Suggest(ctx, provider, sql, len(sql))

	for _, s := range suggestions {
		if s.Category == "Column" && s.Text != "name" {
			t.Errorf("expected only 'name' column with prefix 'na', got %q", s.Text)
		}
	}
}

func TestSuggest_SchemaPrefix(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	sql := "SELECT * FROM auth."
	suggestions := se.Suggest(ctx, provider, sql, len(sql))

	found := false
	for _, s := range suggestions {
		if s.Text == "sessions" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'sessions' table after auth.")
	}
}

func TestSuggest_InsideString(t *testing.T) {
	se, provider := newTestSetup()
	ctx := context.Background()
	sql := "SELECT 'hello "
	suggestions := se.Suggest(ctx, provider, sql, 13) // inside the string
	if len(suggestions) != 0 {
		t.Error("expected no suggestions inside string literal")
	}
}
