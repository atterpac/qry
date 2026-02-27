package autocomplete

import "strings"

// ClauseContext indicates which SQL clause the cursor is positioned in.
type ClauseContext int

const (
	CtxTopLevel      ClauseContext = iota // before any statement keyword
	CtxSelect                             // after SELECT, before FROM
	CtxFrom                               // after FROM
	CtxJoin                               // after JOIN
	CtxOn                                 // after ON (join condition)
	CtxWhere                              // after WHERE
	CtxGroupBy                            // after GROUP BY
	CtxOrderBy                            // after ORDER BY
	CtxHaving                             // after HAVING
	CtxSet                                // after SET in UPDATE
	CtxInsertInto                         // after INSERT INTO
	CtxInsertColumns                      // inside INSERT (...) column list
	CtxUpdate                             // after UPDATE (expecting table)
	CtxDeleteFrom                         // after DELETE FROM
	CtxSchemaPrefix                       // after "schema."
	CtxNone                               // inside string/comment — no suggestions
)

// TableRef represents a table reference found in a SQL statement.
type TableRef struct {
	Schema string
	Name   string
	Alias  string
}

// ParseResult is the output of ParseContext.
type ParseResult struct {
	Context      ClauseContext
	PartialToken string     // incomplete word at cursor for filtering
	PartialStart int        // byte offset where partial starts
	TableRefs    []TableRef // tables/aliases in current statement
	CTENames     []string
	PrecedingDot bool   // cursor is right after "."
	SchemaPrefix string // e.g. "public" if user typed "public."
	AliasTarget  string // table name when completing alias columns (e.g. u. → "users")
}

// ParseContext analyzes SQL text at the given cursor byte position and returns
// context information for autocomplete.
func ParseContext(sql string, cursorPos int) ParseResult {
	if cursorPos > len(sql) {
		cursorPos = len(sql)
	}

	tokens := Tokenize(sql)

	// Check if cursor is inside a string or comment
	for _, t := range tokens {
		if cursorPos > t.Start && cursorPos < t.End {
			if t.Type == TokenString || t.Type == TokenComment {
				return ParseResult{Context: CtxNone}
			}
		}
	}

	// Find current statement by splitting on semicolons
	stmtStart, stmtEnd := findCurrentStatement(tokens, cursorPos)

	// All tokens in the current statement (for table ref / CTE collection)
	var fullStmtTokens []Token
	// Tokens before cursor (for context determination)
	var stmtTokens []Token
	for _, t := range tokens {
		if t.Start >= stmtStart && t.End <= stmtEnd {
			fullStmtTokens = append(fullStmtTokens, t)
			if t.Start < cursorPos {
				stmtTokens = append(stmtTokens, t)
			}
		}
	}

	// Determine partial token at cursor
	partial, partialStart := extractPartial(stmtTokens, cursorPos)

	// Collect CTE names and table references from the FULL statement
	// so that tables defined after the cursor are still available.
	cteNames := collectCTENames(fullStmtTokens)
	tableRefs := collectTableRefs(fullStmtTokens)

	// Determine clause context
	ctx, precedingDot, schemaPrefix, aliasTarget := determineContext(stmtTokens, cursorPos, tableRefs)

	return ParseResult{
		Context:      ctx,
		PartialToken: partial,
		PartialStart: partialStart,
		TableRefs:    tableRefs,
		CTENames:     cteNames,
		PrecedingDot: precedingDot,
		SchemaPrefix: schemaPrefix,
		AliasTarget:  aliasTarget,
	}
}

func findCurrentStatement(tokens []Token, cursorPos int) (int, int) {
	start := 0
	for _, t := range tokens {
		if t.Type == TokenPunctuation && t.Value == ";" {
			if t.End <= cursorPos {
				start = t.End
			}
		}
	}

	end := 0
	if len(tokens) > 0 {
		end = tokens[len(tokens)-1].End
	}
	for _, t := range tokens {
		if t.Type == TokenPunctuation && t.Value == ";" && t.Start >= cursorPos {
			end = t.Start
			break
		}
	}

	return start, end
}

func extractPartial(tokens []Token, cursorPos int) (string, int) {
	if len(tokens) == 0 {
		return "", cursorPos
	}

	last := tokens[len(tokens)-1]
	// Only treat identifiers as partial tokens — keywords (FROM, WHERE, etc.)
	// should never be replaced by a suggestion.
	if last.End == cursorPos && last.Type == TokenIdentifier {
		return last.Value, last.Start
	}

	return "", cursorPos
}

func collectCTENames(tokens []Token) []string {
	var names []string
	for i, t := range tokens {
		if t.Type == TokenKeyword && toUpper(t.Value) == "WITH" {
			// Collect CTE names: WITH name AS (...), name2 AS (...)
			j := i + 1
			for j < len(tokens) {
				// skip whitespace
				for j < len(tokens) && tokens[j].Type == TokenWhitespace {
					j++
				}
				if j >= len(tokens) || tokens[j].Type != TokenIdentifier {
					break
				}
				names = append(names, tokens[j].Value)
				j++
				// skip to closing paren
				depth := 0
				for j < len(tokens) {
					if tokens[j].Type == TokenPunctuation && tokens[j].Value == "(" {
						depth++
					} else if tokens[j].Type == TokenPunctuation && tokens[j].Value == ")" {
						depth--
						if depth == 0 {
							j++
							break
						}
					}
					j++
				}
				// skip whitespace and comma
				for j < len(tokens) && tokens[j].Type == TokenWhitespace {
					j++
				}
				if j < len(tokens) && tokens[j].Type == TokenPunctuation && tokens[j].Value == "," {
					j++
					continue
				}
				break
			}
			break
		}
	}
	return names
}

func collectTableRefs(tokens []Token) []TableRef {
	var refs []TableRef
	nonWS := nonWhitespaceTokens(tokens)

	for i, t := range nonWS {
		upper := toUpper(t.Value)
		if t.Type != TokenKeyword {
			continue
		}

		// FROM table [alias], JOIN table [alias]
		if upper == "FROM" || upper == "JOIN" {
			j := i + 1
			for j < len(nonWS) {
				ref, next := parseTableRef(nonWS, j)
				if ref.Name != "" {
					refs = append(refs, ref)
				}
				j = next
				// Check for comma-separated list (FROM t1, t2)
				if j < len(nonWS) && nonWS[j].Type == TokenPunctuation && nonWS[j].Value == "," {
					j++
					continue
				}
				break
			}
		}

		// UPDATE table
		if upper == "UPDATE" && i+1 < len(nonWS) {
			ref, _ := parseTableRef(nonWS, i+1)
			if ref.Name != "" {
				refs = append(refs, ref)
			}
		}

		// INSERT INTO table
		if upper == "INTO" && i > 0 && toUpper(nonWS[i-1].Value) == "INSERT" && i+1 < len(nonWS) {
			ref, _ := parseTableRef(nonWS, i+1)
			if ref.Name != "" {
				refs = append(refs, ref)
			}
		}
	}

	return refs
}

func parseTableRef(tokens []Token, start int) (TableRef, int) {
	ref := TableRef{}
	if start >= len(tokens) {
		return ref, start
	}

	t := tokens[start]
	if t.Type != TokenIdentifier && t.Type != TokenKeyword {
		return ref, start
	}

	// Check for schema.table pattern
	i := start
	name := t.Value
	i++

	if i < len(tokens) && tokens[i].Type == TokenDot {
		if i+1 < len(tokens) && (tokens[i+1].Type == TokenIdentifier || tokens[i+1].Type == TokenKeyword) {
			ref.Schema = name
			i++ // skip dot
			name = tokens[i].Value
			i++
		} else {
			// Incomplete schema.table (e.g. "public.") — not a valid table ref
			return ref, i
		}
	}

	ref.Name = name

	// Check for alias (next identifier that isn't a keyword, or AS alias)
	if i < len(tokens) {
		if tokens[i].Type == TokenKeyword && toUpper(tokens[i].Value) == "AS" && i+1 < len(tokens) {
			ref.Alias = tokens[i+1].Value
			i += 2
		} else if tokens[i].Type == TokenIdentifier {
			// Implicit alias
			ref.Alias = tokens[i].Value
			i++
		}
	}

	return ref, i
}

func nonWhitespaceTokens(tokens []Token) []Token {
	var result []Token
	for _, t := range tokens {
		if t.Type != TokenWhitespace && t.Type != TokenComment {
			result = append(result, t)
		}
	}
	return result
}

func determineContext(tokens []Token, cursorPos int, tableRefs []TableRef) (ClauseContext, bool, string, string) {
	nonWS := nonWhitespaceTokens(tokens)
	if len(nonWS) == 0 {
		return CtxTopLevel, false, "", ""
	}

	// Check for preceding dot pattern: identifier.partial or identifier.|
	precedingDot := false
	schemaPrefix := ""
	aliasTarget := ""

	// Look at the last few tokens to detect dot patterns
	// Pattern: IDENT DOT [partial] at cursor
	nTokens := len(nonWS)
	lastNonPartial := nTokens - 1

	// If the last token is at the cursor (partial), look before it
	if nTokens > 0 && nonWS[lastNonPartial].End == cursorPos &&
		(nonWS[lastNonPartial].Type == TokenIdentifier || nonWS[lastNonPartial].Type == TokenKeyword) {
		lastNonPartial--
	}

	if lastNonPartial >= 0 && nonWS[lastNonPartial].Type == TokenDot {
		precedingDot = true
		if lastNonPartial > 0 {
			prefix := nonWS[lastNonPartial-1]
			if prefix.Type == TokenIdentifier || prefix.Type == TokenKeyword {
				prefixName := prefix.Value
				// Check if it's an alias
				for _, ref := range tableRefs {
					if strings.EqualFold(ref.Alias, prefixName) || (ref.Alias == "" && strings.EqualFold(ref.Name, prefixName)) {
						aliasTarget = ref.Name
						if ref.Schema != "" {
							schemaPrefix = ref.Schema
						}
						break
					}
				}
				if aliasTarget == "" {
					// Might be a schema prefix
					schemaPrefix = prefixName
					return CtxSchemaPrefix, true, schemaPrefix, ""
				}
			}
		}
	}

	// Walk tokens in reverse to find current clause
	ctx := CtxTopLevel
	parenDepth := 0

	for i := len(nonWS) - 1; i >= 0; i-- {
		t := nonWS[i]

		// Track paren depth (reverse)
		if t.Type == TokenPunctuation && t.Value == ")" {
			parenDepth++
			continue
		}
		if t.Type == TokenPunctuation && t.Value == "(" {
			parenDepth--
			if parenDepth < 0 {
				// We're inside a paren opened before us — check for INSERT columns
				if i > 0 {
					prev := findPrevKeyword(nonWS, i-1)
					if prev != "" && (prev == "INTO" || prev == "TABLE") {
						return CtxInsertColumns, precedingDot, schemaPrefix, aliasTarget
					}
				}
				parenDepth = 0
			}
			continue
		}

		if parenDepth > 0 {
			continue // inside subquery or function call
		}

		if t.Type != TokenKeyword {
			continue
		}

		upper := toUpper(t.Value)

		switch upper {
		case "SELECT":
			ctx = CtxSelect
			return ctx, precedingDot, schemaPrefix, aliasTarget
		case "FROM":
			// Check if preceded by DELETE
			prev := findPrevKeyword(nonWS, i-1)
			if prev == "DELETE" {
				return CtxDeleteFrom, precedingDot, schemaPrefix, aliasTarget
			}
			return CtxFrom, precedingDot, schemaPrefix, aliasTarget
		case "JOIN":
			return CtxJoin, precedingDot, schemaPrefix, aliasTarget
		case "ON":
			return CtxOn, precedingDot, schemaPrefix, aliasTarget
		case "WHERE":
			return CtxWhere, precedingDot, schemaPrefix, aliasTarget
		case "BY":
			prev := findPrevKeyword(nonWS, i-1)
			if prev == "GROUP" {
				return CtxGroupBy, precedingDot, schemaPrefix, aliasTarget
			}
			if prev == "ORDER" {
				return CtxOrderBy, precedingDot, schemaPrefix, aliasTarget
			}
		case "GROUP":
			// GROUP without BY yet — still GROUP BY context
			return CtxGroupBy, precedingDot, schemaPrefix, aliasTarget
		case "ORDER":
			return CtxOrderBy, precedingDot, schemaPrefix, aliasTarget
		case "HAVING":
			return CtxHaving, precedingDot, schemaPrefix, aliasTarget
		case "SET":
			return CtxSet, precedingDot, schemaPrefix, aliasTarget
		case "INTO":
			prev := findPrevKeyword(nonWS, i-1)
			if prev == "INSERT" {
				return CtxInsertInto, precedingDot, schemaPrefix, aliasTarget
			}
		case "UPDATE":
			return CtxUpdate, precedingDot, schemaPrefix, aliasTarget
		case "DELETE":
			return CtxDeleteFrom, precedingDot, schemaPrefix, aliasTarget
		case "INSERT":
			return CtxInsertInto, precedingDot, schemaPrefix, aliasTarget
		}
	}

	return ctx, precedingDot, schemaPrefix, aliasTarget
}

func findPrevKeyword(tokens []Token, from int) string {
	for i := from; i >= 0; i-- {
		if tokens[i].Type == TokenKeyword {
			return toUpper(tokens[i].Value)
		}
		if tokens[i].Type != TokenWhitespace && tokens[i].Type != TokenComment {
			return ""
		}
	}
	return ""
}
