package autocomplete

import "testing"

func TestParseContext_TopLevel(t *testing.T) {
	r := ParseContext("", 0)
	if r.Context != CtxTopLevel {
		t.Errorf("expected CtxTopLevel, got %d", r.Context)
	}
}

func TestParseContext_Select(t *testing.T) {
	sql := "SELECT "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxSelect {
		t.Errorf("expected CtxSelect, got %d", r.Context)
	}
}

func TestParseContext_From(t *testing.T) {
	sql := "SELECT * FROM "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxFrom {
		t.Errorf("expected CtxFrom, got %d", r.Context)
	}
}

func TestParseContext_Where(t *testing.T) {
	sql := "SELECT * FROM users WHERE "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxWhere {
		t.Errorf("expected CtxWhere, got %d", r.Context)
	}
	if len(r.TableRefs) == 0 || r.TableRefs[0].Name != "users" {
		t.Errorf("expected table ref 'users', got %v", r.TableRefs)
	}
}

func TestParseContext_AliasDot(t *testing.T) {
	sql := "SELECT * FROM users u WHERE u."
	r := ParseContext(sql, len(sql))
	if !r.PrecedingDot {
		t.Error("expected PrecedingDot")
	}
	if r.AliasTarget != "users" {
		t.Errorf("expected AliasTarget 'users', got %q", r.AliasTarget)
	}
}

func TestParseContext_SchemaPrefix(t *testing.T) {
	sql := "SELECT * FROM public."
	r := ParseContext(sql, len(sql))
	if r.Context != CtxSchemaPrefix {
		t.Errorf("expected CtxSchemaPrefix, got %d", r.Context)
	}
	if r.SchemaPrefix != "public" {
		t.Errorf("expected SchemaPrefix 'public', got %q", r.SchemaPrefix)
	}
}

func TestParseContext_CTE(t *testing.T) {
	sql := "WITH cte AS (SELECT 1) SELECT * FROM "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxFrom {
		t.Errorf("expected CtxFrom, got %d", r.Context)
	}
	if len(r.CTENames) == 0 || r.CTENames[0] != "cte" {
		t.Errorf("expected CTE name 'cte', got %v", r.CTENames)
	}
}

func TestParseContext_SecondStatement(t *testing.T) {
	sql := "SELECT 1; SELECT * FROM "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxFrom {
		t.Errorf("expected CtxFrom, got %d", r.Context)
	}
}

func TestParseContext_InsideString(t *testing.T) {
	sql := "SELECT 'hello "
	// cursor at position 13 which is inside the string
	r := ParseContext(sql, 13)
	if r.Context != CtxNone {
		t.Errorf("expected CtxNone inside string, got %d", r.Context)
	}
}

func TestParseContext_InsideComment(t *testing.T) {
	sql := "SELECT -- comment"
	r := ParseContext(sql, 12)
	if r.Context != CtxNone {
		t.Errorf("expected CtxNone inside comment, got %d", r.Context)
	}
}

func TestParseContext_Join(t *testing.T) {
	sql := "SELECT * FROM users JOIN "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxJoin {
		t.Errorf("expected CtxJoin, got %d", r.Context)
	}
}

func TestParseContext_GroupBy(t *testing.T) {
	sql := "SELECT name FROM users GROUP BY "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxGroupBy {
		t.Errorf("expected CtxGroupBy, got %d", r.Context)
	}
}

func TestParseContext_OrderBy(t *testing.T) {
	sql := "SELECT name FROM users ORDER BY "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxOrderBy {
		t.Errorf("expected CtxOrderBy, got %d", r.Context)
	}
}

func TestParseContext_PartialToken(t *testing.T) {
	sql := "SELECT na"
	r := ParseContext(sql, len(sql))
	if r.Context != CtxSelect {
		t.Errorf("expected CtxSelect, got %d", r.Context)
	}
	if r.PartialToken != "na" {
		t.Errorf("expected partial 'na', got %q", r.PartialToken)
	}
}

func TestParseContext_Update(t *testing.T) {
	sql := "UPDATE "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxUpdate {
		t.Errorf("expected CtxUpdate, got %d", r.Context)
	}
}

func TestParseContext_DeleteFrom(t *testing.T) {
	sql := "DELETE FROM "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxDeleteFrom {
		t.Errorf("expected CtxDeleteFrom, got %d", r.Context)
	}
}

func TestParseContext_InsertInto(t *testing.T) {
	sql := "INSERT INTO "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxInsertInto {
		t.Errorf("expected CtxInsertInto, got %d", r.Context)
	}
}

func TestParseContext_TableWithAlias(t *testing.T) {
	sql := "SELECT * FROM users AS u WHERE "
	r := ParseContext(sql, len(sql))
	if len(r.TableRefs) == 0 {
		t.Fatal("expected table refs")
	}
	ref := r.TableRefs[0]
	if ref.Name != "users" || ref.Alias != "u" {
		t.Errorf("expected users/u, got %s/%s", ref.Name, ref.Alias)
	}
}

func TestParseContext_MultipleFromTables(t *testing.T) {
	sql := "SELECT * FROM users, orders WHERE "
	r := ParseContext(sql, len(sql))
	if len(r.TableRefs) != 2 {
		t.Fatalf("expected 2 table refs, got %d", len(r.TableRefs))
	}
	if r.TableRefs[0].Name != "users" {
		t.Errorf("expected first ref 'users', got %q", r.TableRefs[0].Name)
	}
	if r.TableRefs[1].Name != "orders" {
		t.Errorf("expected second ref 'orders', got %q", r.TableRefs[1].Name)
	}
}

func TestParseContext_CursorMidSelect_TableAfterCursor(t *testing.T) {
	// Cursor is after SELECT (position 7), table is after FROM
	sql := "SELECT  FROM users"
	r := ParseContext(sql, 7) // cursor right after "SELECT "
	if r.Context != CtxSelect {
		t.Errorf("expected CtxSelect, got %d", r.Context)
	}
	if len(r.TableRefs) == 0 || r.TableRefs[0].Name != "users" {
		t.Errorf("expected table ref 'users' from full statement, got %v", r.TableRefs)
	}
}

func TestParseContext_CursorMidWhere_TableAfterFrom(t *testing.T) {
	sql := "SELECT name FROM users WHERE "
	r := ParseContext(sql, len(sql))
	if r.Context != CtxWhere {
		t.Errorf("expected CtxWhere, got %d", r.Context)
	}
	if len(r.TableRefs) == 0 || r.TableRefs[0].Name != "users" {
		t.Errorf("expected table ref 'users', got %v", r.TableRefs)
	}
}
