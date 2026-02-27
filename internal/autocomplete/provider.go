package autocomplete

import (
	"context"
	"strings"

	"github.com/atterpac/qry/internal/engine"
)

// SuggestionEngine produces context-aware SQL suggestions.
type SuggestionEngine struct {
	Cache  *SchemaCache
	Schema string // default schema (e.g. "public")
}

// NewSuggestionEngine creates an engine with the given cache and default schema.
func NewSuggestionEngine(cache *SchemaCache, defaultSchema string) *SuggestionEngine {
	return &SuggestionEngine{
		Cache:  cache,
		Schema: defaultSchema,
	}
}

// Suggest returns autocomplete suggestions for the given SQL text and cursor position.
func (se *SuggestionEngine) Suggest(ctx context.Context, provider engine.Provider, sql string, cursorPos int) []Suggestion {
	pr := ParseContext(sql, cursorPos)

	if pr.Context == CtxNone {
		return nil
	}

	// If cursor is after alias.dot, show columns of the aliased table
	if pr.PrecedingDot && pr.AliasTarget != "" {
		schema := pr.SchemaPrefix
		if schema == "" {
			schema = se.Schema
		}
		return se.filterPrefix(se.columnSuggestions(ctx, provider, schema, pr.AliasTarget), pr.PartialToken)
	}

	// If cursor is after schema.dot, show tables in that schema
	if pr.Context == CtxSchemaPrefix {
		return se.filterPrefix(se.tableSuggestions(ctx, provider, pr.SchemaPrefix), pr.PartialToken)
	}

	var suggestions []Suggestion

	switch pr.Context {
	case CtxTopLevel:
		// No suggestions — only autocomplete tables/columns, not SQL syntax

	case CtxSelect:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxFrom, CtxJoin:
		suggestions = se.tableSuggestions(ctx, provider, se.Schema)
		suggestions = append(suggestions, se.schemaSuggestions(ctx, provider)...)
		suggestions = append(suggestions, se.cteSuggestions(pr)...)

	case CtxOn:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxWhere:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxGroupBy:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxOrderBy:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxHaving:
		suggestions = se.allColumnSuggestions(ctx, provider, pr)

	case CtxSet:
		if len(pr.TableRefs) > 0 {
			ref := pr.TableRefs[0]
			schema := ref.Schema
			if schema == "" {
				schema = se.Schema
			}
			suggestions = se.columnSuggestions(ctx, provider, schema, ref.Name)
		}

	case CtxInsertInto, CtxUpdate, CtxDeleteFrom:
		suggestions = se.tableSuggestions(ctx, provider, se.Schema)
		suggestions = append(suggestions, se.schemaSuggestions(ctx, provider)...)

	case CtxInsertColumns:
		// Columns of the INSERT target table
		if len(pr.TableRefs) > 0 {
			ref := pr.TableRefs[0]
			schema := ref.Schema
			if schema == "" {
				schema = se.Schema
			}
			suggestions = se.columnSuggestions(ctx, provider, schema, ref.Name)
		}
	}

	return se.filterPrefix(suggestions, pr.PartialToken)
}

func (se *SuggestionEngine) tableSuggestions(ctx context.Context, provider engine.Provider, schema string) []Suggestion {
	tables := se.Cache.Tables(ctx, provider, schema)
	suggestions := make([]Suggestion, 0, len(tables))
	for _, t := range tables {
		suggestions = append(suggestions, Suggestion{
			Text:        t.Name,
			Category:    "Table",
			Description: t.Type,
		})
	}
	return suggestions
}

func (se *SuggestionEngine) columnSuggestions(ctx context.Context, provider engine.Provider, schema, table string) []Suggestion {
	cols := se.Cache.Columns(ctx, provider, schema, table)
	suggestions := make([]Suggestion, 0, len(cols))
	for _, c := range cols {
		desc := c.DataType
		if c.IsPrimaryKey {
			desc += " PK"
		}
		if !c.Nullable {
			desc += " NOT NULL"
		}
		suggestions = append(suggestions, Suggestion{
			Text:        c.Name,
			Category:    "Column",
			Description: desc,
		})
	}
	return suggestions
}

func (se *SuggestionEngine) allColumnSuggestions(ctx context.Context, provider engine.Provider, pr ParseResult) []Suggestion {
	var suggestions []Suggestion
	seen := make(map[string]bool)
	for _, ref := range pr.TableRefs {
		schema := ref.Schema
		if schema == "" {
			schema = se.Schema
		}
		cols := se.columnSuggestions(ctx, provider, schema, ref.Name)
		for _, c := range cols {
			if !seen[c.Text] {
				seen[c.Text] = true
				suggestions = append(suggestions, c)
			}
		}
	}
	return suggestions
}

func (se *SuggestionEngine) schemaSuggestions(ctx context.Context, provider engine.Provider) []Suggestion {
	schemas := se.Cache.Schemas(ctx, provider)
	suggestions := make([]Suggestion, 0, len(schemas))
	for _, s := range schemas {
		suggestions = append(suggestions, Suggestion{
			Text:     s,
			Category: "Schema",
		})
	}
	return suggestions
}

func (se *SuggestionEngine) cteSuggestions(pr ParseResult) []Suggestion {
	suggestions := make([]Suggestion, 0, len(pr.CTENames))
	for _, name := range pr.CTENames {
		suggestions = append(suggestions, Suggestion{
			Text:     name,
			Category: "CTE",
		})
	}
	return suggestions
}

func (se *SuggestionEngine) filterPrefix(suggestions []Suggestion, prefix string) []Suggestion {
	if prefix == "" {
		return suggestions
	}
	lower := strings.ToLower(prefix)
	var filtered []Suggestion
	for _, s := range suggestions {
		if strings.HasPrefix(strings.ToLower(s.Text), lower) {
			filtered = append(filtered, s)
		}
	}
	return filtered
}
