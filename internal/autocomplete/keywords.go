package autocomplete

// Suggestion represents an autocomplete suggestion.
type Suggestion struct {
	Text        string
	InsertText  string // text to insert (defaults to Text if empty)
	Description string
	Category    string // "Keyword", "Table", "Column", "Function", "Type", "Schema", "CTE"
}

// Statement-level keywords
var StatementKeywords = []Suggestion{
	{Text: "SELECT", Category: "Keyword", Description: "Query data"},
	{Text: "INSERT", Category: "Keyword", Description: "Insert rows"},
	{Text: "UPDATE", Category: "Keyword", Description: "Update rows"},
	{Text: "DELETE", Category: "Keyword", Description: "Delete rows"},
	{Text: "WITH", Category: "Keyword", Description: "Common table expression"},
	{Text: "CREATE", Category: "Keyword", Description: "Create object"},
	{Text: "ALTER", Category: "Keyword", Description: "Alter object"},
	{Text: "DROP", Category: "Keyword", Description: "Drop object"},
	{Text: "EXPLAIN", Category: "Keyword", Description: "Show query plan"},
	{Text: "ANALYZE", Category: "Keyword", Description: "Collect statistics"},
	{Text: "TRUNCATE", Category: "Keyword", Description: "Truncate table"},
	{Text: "GRANT", Category: "Keyword", Description: "Grant privileges"},
	{Text: "REVOKE", Category: "Keyword", Description: "Revoke privileges"},
}

// Clause keywords for SELECT context
var SelectKeywords = []Suggestion{
	{Text: "DISTINCT", Category: "Keyword"},
	{Text: "CASE", Category: "Keyword"},
	{Text: "WHEN", Category: "Keyword"},
	{Text: "THEN", Category: "Keyword"},
	{Text: "ELSE", Category: "Keyword"},
	{Text: "END", Category: "Keyword"},
	{Text: "AS", Category: "Keyword"},
	{Text: "FROM", Category: "Keyword"},
}

// Clause keywords for WHERE / ON context
var WhereKeywords = []Suggestion{
	{Text: "AND", Category: "Keyword"},
	{Text: "OR", Category: "Keyword"},
	{Text: "NOT", Category: "Keyword"},
	{Text: "IN", Category: "Keyword"},
	{Text: "LIKE", Category: "Keyword"},
	{Text: "ILIKE", Category: "Keyword"},
	{Text: "BETWEEN", Category: "Keyword"},
	{Text: "IS", Category: "Keyword"},
	{Text: "NULL", Category: "Keyword"},
	{Text: "EXISTS", Category: "Keyword"},
	{Text: "ANY", Category: "Keyword"},
	{Text: "ALL", Category: "Keyword"},
	{Text: "TRUE", Category: "Keyword"},
	{Text: "FALSE", Category: "Keyword"},
	{Text: "CASE", Category: "Keyword"},
}

// ORDER BY keywords
var OrderByKeywords = []Suggestion{
	{Text: "ASC", Category: "Keyword"},
	{Text: "DESC", Category: "Keyword"},
	{Text: "NULLS FIRST", Category: "Keyword"},
	{Text: "NULLS LAST", Category: "Keyword"},
}

// JOIN modifiers
var JoinKeywords = []Suggestion{
	{Text: "INNER JOIN", Category: "Keyword"},
	{Text: "LEFT JOIN", Category: "Keyword"},
	{Text: "RIGHT JOIN", Category: "Keyword"},
	{Text: "FULL OUTER JOIN", Category: "Keyword"},
	{Text: "CROSS JOIN", Category: "Keyword"},
}

// Aggregate functions
var AggregateFunctions = []Suggestion{
	{Text: "COUNT", Category: "Function", InsertText: "COUNT(", Description: "Count rows"},
	{Text: "SUM", Category: "Function", InsertText: "SUM(", Description: "Sum values"},
	{Text: "AVG", Category: "Function", InsertText: "AVG(", Description: "Average values"},
	{Text: "MIN", Category: "Function", InsertText: "MIN(", Description: "Minimum value"},
	{Text: "MAX", Category: "Function", InsertText: "MAX(", Description: "Maximum value"},
	{Text: "ARRAY_AGG", Category: "Function", InsertText: "ARRAY_AGG(", Description: "Aggregate into array"},
	{Text: "STRING_AGG", Category: "Function", InsertText: "STRING_AGG(", Description: "Concatenate strings"},
	{Text: "BOOL_AND", Category: "Function", InsertText: "BOOL_AND(", Description: "Logical AND aggregate"},
	{Text: "BOOL_OR", Category: "Function", InsertText: "BOOL_OR(", Description: "Logical OR aggregate"},
}

// Common functions
var CommonFunctions = []Suggestion{
	{Text: "COALESCE", Category: "Function", InsertText: "COALESCE(", Description: "First non-null value"},
	{Text: "NULLIF", Category: "Function", InsertText: "NULLIF(", Description: "Return null if equal"},
	{Text: "GREATEST", Category: "Function", InsertText: "GREATEST(", Description: "Largest value"},
	{Text: "LEAST", Category: "Function", InsertText: "LEAST(", Description: "Smallest value"},
	{Text: "NOW", Category: "Function", InsertText: "NOW()", Description: "Current timestamp"},
	{Text: "CURRENT_TIMESTAMP", Category: "Function", Description: "Current timestamp"},
	{Text: "CURRENT_DATE", Category: "Function", Description: "Current date"},
	{Text: "EXTRACT", Category: "Function", InsertText: "EXTRACT(", Description: "Extract date part"},
	{Text: "DATE_TRUNC", Category: "Function", InsertText: "DATE_TRUNC(", Description: "Truncate date"},
	{Text: "LENGTH", Category: "Function", InsertText: "LENGTH(", Description: "String length"},
	{Text: "UPPER", Category: "Function", InsertText: "UPPER(", Description: "Uppercase"},
	{Text: "LOWER", Category: "Function", InsertText: "LOWER(", Description: "Lowercase"},
	{Text: "TRIM", Category: "Function", InsertText: "TRIM(", Description: "Trim whitespace"},
	{Text: "SUBSTRING", Category: "Function", InsertText: "SUBSTRING(", Description: "Extract substring"},
	{Text: "CONCAT", Category: "Function", InsertText: "CONCAT(", Description: "Concatenate strings"},
	{Text: "REPLACE", Category: "Function", InsertText: "REPLACE(", Description: "Replace substring"},
	{Text: "SPLIT_PART", Category: "Function", InsertText: "SPLIT_PART(", Description: "Split and get part"},
	{Text: "TO_CHAR", Category: "Function", InsertText: "TO_CHAR(", Description: "Format to string"},
	{Text: "TO_DATE", Category: "Function", InsertText: "TO_DATE(", Description: "Parse date string"},
	{Text: "TO_NUMBER", Category: "Function", InsertText: "TO_NUMBER(", Description: "Parse number string"},
	{Text: "GENERATE_SERIES", Category: "Function", InsertText: "GENERATE_SERIES(", Description: "Generate series"},
	{Text: "ROW_NUMBER", Category: "Function", InsertText: "ROW_NUMBER() OVER (", Description: "Row number window"},
	{Text: "RANK", Category: "Function", InsertText: "RANK() OVER (", Description: "Rank window"},
	{Text: "DENSE_RANK", Category: "Function", InsertText: "DENSE_RANK() OVER (", Description: "Dense rank window"},
	{Text: "LAG", Category: "Function", InsertText: "LAG(", Description: "Previous row value"},
	{Text: "LEAD", Category: "Function", InsertText: "LEAD(", Description: "Next row value"},
}

// Data types for CREATE/CAST
var DataTypes = []Suggestion{
	{Text: "integer", Category: "Type"},
	{Text: "bigint", Category: "Type"},
	{Text: "smallint", Category: "Type"},
	{Text: "serial", Category: "Type"},
	{Text: "bigserial", Category: "Type"},
	{Text: "text", Category: "Type"},
	{Text: "varchar", Category: "Type"},
	{Text: "char", Category: "Type"},
	{Text: "boolean", Category: "Type"},
	{Text: "timestamp", Category: "Type"},
	{Text: "timestamptz", Category: "Type"},
	{Text: "date", Category: "Type"},
	{Text: "time", Category: "Type"},
	{Text: "interval", Category: "Type"},
	{Text: "uuid", Category: "Type"},
	{Text: "jsonb", Category: "Type"},
	{Text: "json", Category: "Type"},
	{Text: "numeric", Category: "Type"},
	{Text: "real", Category: "Type"},
	{Text: "double precision", Category: "Type"},
	{Text: "bytea", Category: "Type"},
	{Text: "inet", Category: "Type"},
	{Text: "cidr", Category: "Type"},
	{Text: "macaddr", Category: "Type"},
	{Text: "array", Category: "Type"},
}
