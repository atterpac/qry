package autocomplete

import (
	"context"
	"sync"
	"time"

	"github.com/atterpac/qry/internal/engine"
)

// SchemaCache lazily caches database metadata for autocomplete.
type SchemaCache struct {
	mu        sync.RWMutex
	schemas   []string
	tables    map[string][]engine.TableInfo  // key: schema
	columns   map[string][]engine.ColumnInfo // key: "schema.table"
	lastFetch map[string]time.Time
	ttl       time.Duration
}

// NewSchemaCache creates a cache with the given TTL.
func NewSchemaCache(ttl time.Duration) *SchemaCache {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &SchemaCache{
		tables:    make(map[string][]engine.TableInfo),
		columns:   make(map[string][]engine.ColumnInfo),
		lastFetch: make(map[string]time.Time),
		ttl:       ttl,
	}
}

// Schemas returns cached schema names, fetching if stale.
func (c *SchemaCache) Schemas(ctx context.Context, provider engine.Provider) []string {
	c.mu.RLock()
	if c.schemas != nil && time.Since(c.lastFetch["schemas"]) < c.ttl {
		result := c.schemas
		c.mu.RUnlock()
		return result
	}
	c.mu.RUnlock()

	schemas, err := provider.ListSchemas(ctx, "")
	if err != nil {
		return nil
	}

	c.mu.Lock()
	c.schemas = schemas
	c.lastFetch["schemas"] = time.Now()
	c.mu.Unlock()
	return schemas
}

// Tables returns cached tables for a schema, fetching if stale.
func (c *SchemaCache) Tables(ctx context.Context, provider engine.Provider, schema string) []engine.TableInfo {
	key := "tables:" + schema
	c.mu.RLock()
	if tables, ok := c.tables[schema]; ok && time.Since(c.lastFetch[key]) < c.ttl {
		c.mu.RUnlock()
		return tables
	}
	c.mu.RUnlock()

	tables, err := provider.ListTables(ctx, schema)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	c.tables[schema] = tables
	c.lastFetch[key] = time.Now()
	c.mu.Unlock()
	return tables
}

// Columns returns cached columns for a table, fetching if stale.
func (c *SchemaCache) Columns(ctx context.Context, provider engine.Provider, schema, table string) []engine.ColumnInfo {
	key := schema + "." + table
	c.mu.RLock()
	if cols, ok := c.columns[key]; ok && time.Since(c.lastFetch["cols:"+key]) < c.ttl {
		c.mu.RUnlock()
		return cols
	}
	c.mu.RUnlock()

	cols, err := provider.DescribeTable(ctx, schema, table)
	if err != nil {
		return nil
	}

	c.mu.Lock()
	c.columns[key] = cols
	c.lastFetch["cols:"+key] = time.Now()
	c.mu.Unlock()
	return cols
}

// Invalidate clears all cached data.
func (c *SchemaCache) Invalidate() {
	c.mu.Lock()
	c.schemas = nil
	c.tables = make(map[string][]engine.TableInfo)
	c.columns = make(map[string][]engine.ColumnInfo)
	c.lastFetch = make(map[string]time.Time)
	c.mu.Unlock()
}

// Warm pre-fetches schemas and tables for the given default schema.
func (c *SchemaCache) Warm(ctx context.Context, provider engine.Provider, defaultSchema string) {
	c.Schemas(ctx, provider)
	if defaultSchema != "" {
		c.Tables(ctx, provider, defaultSchema)
	}
}
