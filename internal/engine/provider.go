package engine

import (
	"context"
	"fmt"

	"github.com/atterpac/qry/internal/config"
)

// EngineCapabilities describes what a database engine supports.
type EngineCapabilities struct {
	HasSchemas        bool
	HasDatabases      bool
	HasNamespaces     bool
	HasRecordLinks    bool
	HasGraphQueries   bool
	SupportsReturning bool
	IdentifierQuote   string
}

// TableInfo describes a database table.
type TableInfo struct {
	Schema string
	Name   string
	Type   string // "table", "view", etc.
}

// ColumnInfo describes a column in a table.
type ColumnInfo struct {
	Name         string
	DataType     string
	Nullable     bool
	IsPrimaryKey bool
	Default      string
	Extra        string // auto_increment, generated, etc.
}

// SearchFilter represents a single search term, optionally targeting a specific column.
type SearchFilter struct {
	Column string // Empty means search all columns
	Value  string
}

// CellChange describes a single cell modification from the DataGrid changeset.
type CellChange struct {
	Column   string
	OldValue string
	NewValue string
}

// QueryResult holds the result of executing a query.
type QueryResult struct {
	Columns  []string
	Rows     [][]string
	RowCount int
	Duration fmt.Stringer
	Message  string // for non-SELECT queries (e.g., "5 rows affected")
}

// ForeignKeyInfo describes a foreign key relationship for a table.
type ForeignKeyInfo struct {
	Column           string // Column on the current table
	ReferencedSchema string // Schema of the related table
	ReferencedTable  string // Related table name
	ReferencedColumn string // Column on the related table
	IsInbound        bool   // true = another table references this table's column
}

// Provider abstracts all database engines behind one interface.
type Provider interface {
	// Connection lifecycle
	Connect(ctx context.Context, cfg config.ConnectionConfig) error
	Close() error
	Ping(ctx context.Context) error
	IsConnected() bool
	EngineType() config.EngineType
	ServerVersion(ctx context.Context) (string, error)
	Capabilities() EngineCapabilities

	// Schema browsing
	ListDatabases(ctx context.Context) ([]string, error)
	ListSchemas(ctx context.Context, database string) ([]string, error)
	ListTables(ctx context.Context, schema string) ([]TableInfo, error)
	DescribeTable(ctx context.Context, schema, table string) ([]ColumnInfo, error)
	GetForeignKeys(ctx context.Context, schema, table string) ([]ForeignKeyInfo, error)

	// Query execution
	ExecuteQuery(ctx context.Context, query string) (*QueryResult, error)
	ExecuteArgs(ctx context.Context, query string, args []any) (*QueryResult, error)

	// ExplainPlan runs the engine's EXPLAIN variant and returns a parsed plan.
	ExplainPlan(ctx context.Context, sql string) (*PlanResult, error)

	// SQLDialect groups the engine's SQL string-generation methods.
	SQLDialect
}

// SQLDialect groups the engine-specific SQL string generation: search clause
// construction, DML built from a DataGrid changeset, and identifier quoting.
type SQLDialect interface {
	// Search clause generation
	BuildSearchClause(columns []string, filters []SearchFilter) string

	// SQL generation from DataGrid changeset
	BuildUpdate(schema, table string, pkCols []string, changes []CellChange, pkValues map[string]string) (string, []any, error)
	BuildInsert(schema, table string, columns []string, values []string) (string, []any, error)
	BuildDelete(schema, table string, pkCols []string, pkValues map[string]string) (string, []any, error)
	QuoteIdentifier(name string) string
}
