package headless

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atterpac/qry/internal/engine"
)

// Format formats a QueryResult into the specified output format.
func Format(result *engine.QueryResult, format string) string {
	if result == nil || len(result.Columns) == 0 {
		if result != nil && result.Message != "" {
			return result.Message
		}
		return ""
	}

	switch format {
	case "csv":
		return formatCSV(result)
	case "json":
		return formatJSON(result)
	case "tsv":
		return formatTSV(result)
	case "table":
		return formatTable(result)
	case "sql":
		return formatSQL(result)
	default:
		return formatTable(result)
	}
}

func formatCSV(r *engine.QueryResult) string {
	var buf strings.Builder
	w := csv.NewWriter(&buf)
	w.Write(r.Columns)
	for _, row := range r.Rows {
		w.Write(row)
	}
	w.Flush()
	return buf.String()
}

func formatJSON(r *engine.QueryResult) string {
	var rows []map[string]string
	for _, row := range r.Rows {
		m := make(map[string]string, len(r.Columns))
		for i, col := range r.Columns {
			if i < len(row) {
				m[col] = row[i]
			}
		}
		rows = append(rows, m)
	}
	if rows == nil {
		rows = []map[string]string{}
	}
	data, _ := json.MarshalIndent(rows, "", "  ")
	return string(data)
}

func formatTSV(r *engine.QueryResult) string {
	var buf strings.Builder
	buf.WriteString(strings.Join(r.Columns, "\t"))
	buf.WriteByte('\n')
	for _, row := range r.Rows {
		buf.WriteString(strings.Join(row, "\t"))
		buf.WriteByte('\n')
	}
	return buf.String()
}

func formatTable(r *engine.QueryResult) string {
	// Calculate column widths
	widths := make([]int, len(r.Columns))
	for i, col := range r.Columns {
		widths[i] = len(col)
	}
	for _, row := range r.Rows {
		for i, val := range row {
			if i < len(widths) && len(val) > widths[i] {
				widths[i] = len(val)
			}
		}
	}

	// Cap widths
	for i := range widths {
		if widths[i] > 50 {
			widths[i] = 50
		}
	}

	var buf strings.Builder

	// Header
	var headerParts []string
	for i, col := range r.Columns {
		headerParts = append(headerParts, padOrTrunc(col, widths[i]))
	}
	buf.WriteString(strings.Join(headerParts, " | "))
	buf.WriteByte('\n')

	// Separator
	var sepParts []string
	for _, w := range widths {
		sepParts = append(sepParts, strings.Repeat("-", w))
	}
	buf.WriteString(strings.Join(sepParts, "-+-"))
	buf.WriteByte('\n')

	// Rows
	for _, row := range r.Rows {
		var parts []string
		for i, val := range row {
			if i < len(widths) {
				parts = append(parts, padOrTrunc(val, widths[i]))
			}
		}
		buf.WriteString(strings.Join(parts, " | "))
		buf.WriteByte('\n')
	}

	buf.WriteString(fmt.Sprintf("(%d rows)\n", len(r.Rows)))
	return buf.String()
}

func formatSQL(r *engine.QueryResult) string {
	var stmts []string
	for _, row := range r.Rows {
		var vals []string
		for _, v := range row {
			if strings.EqualFold(v, "NULL") || v == "" {
				vals = append(vals, "NULL")
			} else {
				vals = append(vals, "'"+strings.ReplaceAll(v, "'", "''")+"'")
			}
		}
		stmts = append(stmts, fmt.Sprintf("INSERT INTO _table_ (%s) VALUES (%s);",
			strings.Join(r.Columns, ", "), strings.Join(vals, ", ")))
	}
	return strings.Join(stmts, "\n")
}

func padOrTrunc(s string, width int) string {
	if len(s) > width {
		return s[:width-1] + "…"
	}
	return s + strings.Repeat(" ", width-len(s))
}
