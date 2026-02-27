package autocomplete

// TokenType classifies a lexical token from SQL text.
type TokenType int

const (
	TokenKeyword     TokenType = iota // SQL keyword (SELECT, FROM, etc.)
	TokenIdentifier                   // unquoted or double-quoted identifier
	TokenOperator                     // =, <>, !=, >=, <=, ::
	TokenNumber                       // numeric literal
	TokenString                       // single-quoted string literal
	TokenPunctuation                  // (, ), ;, ,
	TokenStar                         // *
	TokenWhitespace                   // spaces, tabs, newlines
	TokenComment                      // -- or /* */ comment
	TokenDot                          // .
)

// Token represents a single lexical unit in SQL text.
type Token struct {
	Type  TokenType
	Value string
	Start int // byte offset (inclusive)
	End   int // byte offset (exclusive)
}

// sqlKeywords is the set of SQL keywords for classification.
var sqlKeywords = map[string]bool{
	"SELECT": true, "FROM": true, "WHERE": true, "INSERT": true, "INTO": true,
	"UPDATE": true, "DELETE": true, "SET": true, "VALUES": true, "CREATE": true,
	"ALTER": true, "DROP": true, "TABLE": true, "INDEX": true, "VIEW": true,
	"WITH": true, "AS": true, "ON": true, "JOIN": true, "INNER": true,
	"LEFT": true, "RIGHT": true, "FULL": true, "OUTER": true, "CROSS": true,
	"NATURAL": true, "AND": true, "OR": true, "NOT": true, "IN": true,
	"LIKE": true, "ILIKE": true, "BETWEEN": true, "IS": true, "NULL": true,
	"EXISTS": true, "ANY": true, "ALL": true, "CASE": true, "WHEN": true,
	"THEN": true, "ELSE": true, "END": true, "TRUE": true, "FALSE": true,
	"GROUP": true, "BY": true, "ORDER": true, "HAVING": true, "LIMIT": true,
	"OFFSET": true, "UNION": true, "INTERSECT": true, "EXCEPT": true,
	"RETURNING": true, "DISTINCT": true, "ASC": true, "DESC": true,
	"NULLS": true, "FIRST": true, "LAST": true, "EXPLAIN": true,
	"ANALYZE": true, "TRUNCATE": true, "GRANT": true, "REVOKE": true,
	"CASCADE": true, "RESTRICT": true, "IF": true, "SCHEMA": true,
	"DATABASE": true, "TYPE": true, "ENUM": true, "SEQUENCE": true,
	"CONSTRAINT": true, "PRIMARY": true, "KEY": true, "FOREIGN": true,
	"REFERENCES": true, "UNIQUE": true, "CHECK": true, "DEFAULT": true,
	"ADD": true, "COLUMN": true, "RENAME": true, "TO": true,
}

// Tokenize performs a single-pass tokenization of SQL text.
func Tokenize(sql string) []Token {
	var tokens []Token
	i := 0
	n := len(sql)

	for i < n {
		ch := sql[i]

		// Whitespace
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			start := i
			for i < n && (sql[i] == ' ' || sql[i] == '\t' || sql[i] == '\n' || sql[i] == '\r') {
				i++
			}
			tokens = append(tokens, Token{TokenWhitespace, sql[start:i], start, i})
			continue
		}

		// Single-line comment: --
		if ch == '-' && i+1 < n && sql[i+1] == '-' {
			start := i
			i += 2
			for i < n && sql[i] != '\n' {
				i++
			}
			tokens = append(tokens, Token{TokenComment, sql[start:i], start, i})
			continue
		}

		// Block comment: /* ... */
		if ch == '/' && i+1 < n && sql[i+1] == '*' {
			start := i
			i += 2
			for i+1 < n {
				if sql[i] == '*' && sql[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i >= n && !(i >= 2 && sql[i-2] == '*' && sql[i-1] == '/') {
				// unterminated comment — consume rest
				i = n
			}
			tokens = append(tokens, Token{TokenComment, sql[start:i], start, i})
			continue
		}

		// Single-quoted string
		if ch == '\'' {
			start := i
			i++
			for i < n {
				if sql[i] == '\'' {
					i++
					if i < n && sql[i] == '\'' {
						i++ // escaped ''
						continue
					}
					break
				}
				i++
			}
			tokens = append(tokens, Token{TokenString, sql[start:i], start, i})
			continue
		}

		// Dollar-quoted string: $tag$...$tag$
		if ch == '$' {
			if tag, tagEnd := dollarTag(sql, i); tagEnd > i {
				start := i
				i = tagEnd
				// Find closing tag
				for i < n {
					if sql[i] == '$' {
						if closeTag, closeEnd := dollarTag(sql, i); closeEnd > i && tag == closeTag {
							i = closeEnd
							break
						}
					}
					i++
				}
				tokens = append(tokens, Token{TokenString, sql[start:i], start, i})
				continue
			}
		}

		// Double-quoted identifier
		if ch == '"' {
			start := i
			i++
			for i < n {
				if sql[i] == '"' {
					i++
					if i < n && sql[i] == '"' {
						i++ // escaped ""
						continue
					}
					break
				}
				i++
			}
			tokens = append(tokens, Token{TokenIdentifier, sql[start:i], start, i})
			continue
		}

		// Dot
		if ch == '.' {
			tokens = append(tokens, Token{TokenDot, ".", i, i + 1})
			i++
			continue
		}

		// Star
		if ch == '*' {
			tokens = append(tokens, Token{TokenStar, "*", i, i + 1})
			i++
			continue
		}

		// Punctuation: (, ), ;, ,
		if ch == '(' || ch == ')' || ch == ';' || ch == ',' {
			tokens = append(tokens, Token{TokenPunctuation, string(ch), i, i + 1})
			i++
			continue
		}

		// Operators: ::, >=, <=, <>, !=, =, <, >, +, -, /, %
		if ch == ':' && i+1 < n && sql[i+1] == ':' {
			tokens = append(tokens, Token{TokenOperator, "::", i, i + 2})
			i += 2
			continue
		}
		if isOperatorStart(ch) {
			start := i
			// Two-char operators
			if i+1 < n {
				two := sql[i : i+2]
				if two == ">=" || two == "<=" || two == "<>" || two == "!=" {
					i += 2
					tokens = append(tokens, Token{TokenOperator, two, start, i})
					continue
				}
			}
			tokens = append(tokens, Token{TokenOperator, string(ch), i, i + 1})
			i++
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			start := i
			for i < n && ((sql[i] >= '0' && sql[i] <= '9') || sql[i] == '.') {
				i++
			}
			tokens = append(tokens, Token{TokenNumber, sql[start:i], start, i})
			continue
		}

		// Identifiers / keywords
		if isIdentStart(ch) {
			start := i
			for i < n && isIdentCont(sql[i]) {
				i++
			}
			word := sql[start:i]
			tt := TokenIdentifier
			if sqlKeywords[toUpper(word)] {
				tt = TokenKeyword
			}
			tokens = append(tokens, Token{tt, word, start, i})
			continue
		}

		// Unknown character — skip
		i++
	}

	return tokens
}

func dollarTag(sql string, pos int) (string, int) {
	if pos >= len(sql) || sql[pos] != '$' {
		return "", pos
	}
	i := pos + 1
	// tag can be empty ($$ ... $$) or alphanumeric
	for i < len(sql) && (isIdentCont(sql[i])) {
		i++
	}
	if i < len(sql) && sql[i] == '$' {
		return sql[pos : i+1], i + 1
	}
	return "", pos
}

func isOperatorStart(ch byte) bool {
	return ch == '=' || ch == '<' || ch == '>' || ch == '!' || ch == '+' || ch == '-' || ch == '/' || ch == '%'
}

func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isIdentCont(ch byte) bool {
	return isIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func toUpper(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		b[i] = c
	}
	return string(b)
}
