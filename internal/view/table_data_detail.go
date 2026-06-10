package view

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/theme"

	"github.com/atterpac/qry/internal/config"
)

func (t *TableData) showCellDetail() {
	pos := t.grid.GetCursor()
	if pos.Col < 0 || pos.Col >= len(t.resultCols) {
		return
	}

	colName := t.resultCols[pos.Col]
	value := t.grid.GetCellValue(pos)

	tv := core.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWordWrap(true)
	tv.SetText(value)
	tv.SetScrollable(true)

	modal := components.NewModal(components.ModalConfig{
		Title:    colName,
		Width:    70,
		Height:   20,
		Backdrop: true,
	}).SetContent(tv).
		SetHints([]components.KeyHint{
			{Key: "Esc", Description: "Close"},
		})

	t.app.app.Pages().Push(modal)
}

func (t *TableData) showSchemaOverlay() {
	var buf strings.Builder
	fmt.Fprintf(&buf, "[::b]Schema: %s[::-]\n\n", t.table)

	for _, col := range t.columns {
		tags := ""
		if col.IsPrimaryKey {
			tags += fmt.Sprintf(" [%s]PK[-]", theme.TagWarning())
		}
		if !col.Nullable {
			tags += fmt.Sprintf(" [%s]NOT NULL[-]", theme.TagError())
		}
		if col.Default != "" {
			tags += fmt.Sprintf(" [%s]DEFAULT %s[-]", theme.TagFgDim(), col.Default)
		}
		if col.Extra != "" {
			tags += fmt.Sprintf(" [%s]%s[-]", theme.TagFgDim(), col.Extra)
		}
		fmt.Fprintf(&buf, "  [::b]%s[::-]  %s%s\n", col.Name, col.DataType, tags)
	}

	tv := core.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWordWrap(true)
	tv.SetText(buf.String())
	tv.SetScrollable(true)

	modal := components.NewModal(components.ModalConfig{
		Title:    "Schema: " + t.table,
		Width:    70,
		Height:   min(len(t.columns)+6, 30),
		Backdrop: true,
	}).SetContent(tv).
		SetHints([]components.KeyHint{
			{Key: "Esc", Description: "Close"},
		})

	t.app.app.Pages().Push(modal)
}

func (t *TableData) saveBookmark() {
	defaultName := t.table
	if t.schema != "" {
		defaultName = t.schema + "." + t.table
	}

	form := components.NewFormBuilder().
		Text("name", "Bookmark name").Value(defaultName).Done().
		OnSubmit(func(values map[string]any) {
			name, _ := values["name"].(string)
			if name == "" {
				t.app.ShowWarning("Name cannot be empty")
				return
			}
			added := t.app.Config().AddBookmark(config.Bookmark{
				Type:   "table",
				Name:   t.table,
				Schema: t.schema,
			})
			if added {
				go t.app.Config().Save()
				t.app.ShowSuccess(fmt.Sprintf("Bookmarked: %s", name))
			} else {
				t.app.ShowInfo("Bookmark already exists")
			}
			t.app.app.Pages().Pop()
		}).
		OnCancel(func() {
			t.app.app.Pages().Pop()
		}).
		Build()

	modal := components.NewModal(components.ModalConfig{
		Title:    "Save Bookmark",
		Width:    50,
		Height:   8,
		Backdrop: true,
	}).SetContent(form).
		SetFocusOnShow(form)

	t.app.app.Pages().Push(modal)
}

func (t *TableData) showBookmarkPicker() {
	t.app.showBookmarkPicker()
}

func (t *TableData) rebuildLayout(showEmpty bool) {
	t.Clear()
	t.gridFlex.Clear()

	if t.detailVisible {
		// Outer flex is columns: [gridFlex | detailPanel]
		t.SetDirection(core.Row)

		if showEmpty {
			t.gridFlex.AddItem(t.emptyState, 0, 1, true)
		} else {
			t.gridFlex.AddItem(t.grid, 0, 1, true)
		}
		t.gridFlex.AddItem(t.statusBar, 1, 0, false)

		t.AddItem(t.gridFlex, 0, 3, true)
		t.AddItem(t.detailPanel, 0, 2, false)
	} else {
		// Outer flex is rows: [grid, statusBar] (original layout)
		t.SetDirection(core.Column)

		if showEmpty {
			t.AddItem(t.emptyState, 0, 1, true)
		} else {
			t.AddItem(t.grid, 0, 1, true)
		}
		t.AddItem(t.statusBar, 1, 0, false)
	}
}

func (t *TableData) toggleDetailPanel() {
	t.detailVisible = !t.detailVisible
	if t.detailVisible {
		t.updateDetailPanel()
	}
	showEmpty := t.source.RowCount() == 0
	t.rebuildLayout(showEmpty)
}

func (t *TableData) updateDetailPanel() {
	row := t.grid.GetCursorRow()
	if row == nil {
		t.detailText.SetText("")
		t.detailPanel.SetTitle("Row Detail")
		return
	}

	rowIdx := t.grid.GetCursorRowIndex()
	t.detailPanel.SetTitle(fmt.Sprintf("Row %d", rowIdx+1))

	accent := theme.TagAccent()
	var buf strings.Builder
	buf.WriteString("{\n")
	first := true
	for _, col := range t.resultCols {
		v, ok := row[col]
		if !ok {
			continue
		}
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		keyJSON, _ := json.Marshal(col)
		fmt.Fprintf(&buf, "  [%s]%s[-]: ", accent, string(keyJSON))
		formatDetailValue(&buf, v, accent, "  ")
	}
	buf.WriteString("\n}")
	t.detailText.SetText(buf.String())
	t.detailText.ScrollTo(0, 0)
}

// formatDetailValue writes a syntax-highlighted value to buf. If the string is
// valid JSON (object or array), it is pretty-printed with nested highlighting.
// Go map literals (map[k:v ...]) are also detected and formatted.
// Otherwise it is written as a JSON-encoded string.
func formatDetailValue(buf *strings.Builder, v, accent, indent string) {
	trimmed := strings.TrimSpace(v)
	// Try JSON first (jsonb columns, arrays).
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var parsed any
		if json.Unmarshal([]byte(trimmed), &parsed) == nil {
			writeDetailJSON(buf, parsed, accent, indent)
			return
		}
	}
	// Try Go map literal: map[k1:v1 k2:v2]
	if parsed, ok := parseGoMap(trimmed); ok {
		writeDetailJSON(buf, parsed, accent, indent)
		return
	}
	// Plain scalar — quote it.
	valJSON, _ := json.Marshal(v)
	buf.Write(valJSON)
}

// parseGoMap attempts to parse a Go-style map literal like "map[k:v k2:v2]".
// Supports nested maps and slices (e.g. map[k:map[a:b]] or [v1 v2]).
func parseGoMap(s string) (any, bool) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "map[") && strings.HasSuffix(s, "]") {
		inner := s[4 : len(s)-1]
		result := make(map[string]any)
		for len(inner) > 0 {
			inner = strings.TrimLeft(inner, " ")
			if len(inner) == 0 {
				break
			}
			colonIdx := strings.Index(inner, ":")
			if colonIdx < 0 {
				return nil, false
			}
			key := inner[:colonIdx]
			inner = inner[colonIdx+1:]

			val, rest, ok := extractGoValue(inner)
			if !ok {
				return nil, false
			}
			result[key] = val
			inner = rest
		}
		return result, true
	}
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return parseGoSlice(s)
	}
	return nil, false
}

// parseGoSlice parses a Go-style slice literal like "[v1 v2 v3]".
func parseGoSlice(s string) (any, bool) {
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, false
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	if inner == "" {
		return []any{}, true
	}
	var items []any
	for len(inner) > 0 {
		inner = strings.TrimLeft(inner, " ")
		if len(inner) == 0 {
			break
		}
		val, rest, ok := extractGoValue(inner)
		if !ok {
			return nil, false
		}
		items = append(items, val)
		inner = rest
	}
	return items, true
}

// extractGoValue extracts the next value from a Go map/slice literal string.
// Handles nested map[...], [...], and plain tokens delimited by space or end.
func extractGoValue(s string) (any, string, bool) {
	s = strings.TrimLeft(s, " ")
	if len(s) == 0 {
		return nil, "", false
	}
	// Nested map
	if strings.HasPrefix(s, "map[") {
		end := findMatchingBracket(s, 3)
		if end < 0 {
			return nil, "", false
		}
		parsed, ok := parseGoMap(s[:end+1])
		if !ok {
			return nil, "", false
		}
		return parsed, strings.TrimLeft(s[end+1:], " "), true
	}
	// Nested slice
	if s[0] == '[' {
		end := findMatchingBracket(s, 0)
		if end < 0 {
			return nil, "", false
		}
		parsed, ok := parseGoSlice(s[:end+1])
		if !ok {
			return nil, "", false
		}
		return parsed, strings.TrimLeft(s[end+1:], " "), true
	}
	// Plain token: read until space or end, respecting nested brackets
	i := 0
	depth := 0
	for i < len(s) {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth == 0 {
				return s[:i], s[i:], true
			}
			depth--
		case ' ':
			if depth == 0 {
				return s[:i], s[i:], true
			}
		}
		i++
	}
	return s, "", true
}

// findMatchingBracket finds the ']' matching the '[' at position start.
func findMatchingBracket(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// writeDetailJSON recursively writes a syntax-highlighted JSON value.
func writeDetailJSON(buf *strings.Builder, v any, accent, indent string) {
	nextIndent := indent + "  "
	switch val := v.(type) {
	case map[string]any:
		buf.WriteString("{\n")
		first := true
		for k, child := range val {
			if !first {
				buf.WriteString(",\n")
			}
			first = false
			keyJSON, _ := json.Marshal(k)
			buf.WriteString(nextIndent)
			fmt.Fprintf(buf, "[%s]%s[-]: ", accent, string(keyJSON))
			writeDetailJSON(buf, child, accent, nextIndent)
		}
		buf.WriteString("\n")
		buf.WriteString(indent)
		buf.WriteString("}")
	case []any:
		buf.WriteString("[\n")
		for i, child := range val {
			if i > 0 {
				buf.WriteString(",\n")
			}
			buf.WriteString(nextIndent)
			writeDetailJSON(buf, child, accent, nextIndent)
		}
		buf.WriteString("\n")
		buf.WriteString(indent)
		buf.WriteString("]")
	default:
		encoded, _ := json.Marshal(val)
		buf.Write(encoded)
	}
}
