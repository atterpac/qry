package view

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/atterpac/dado/clipboard"
	"github.com/atterpac/dado/components"

	"github.com/atterpac/qry/internal/engine"
)

func (t *TableData) yankRow() {
	t.yankBuffer = t.grid.GetCursorRow()
	if t.yankBuffer == nil {
		t.app.ShowWarning("No row to yank")
		return
	}
	// Copy as formatted JSON to system clipboard (preserving column order)
	var buf strings.Builder
	buf.WriteString("{\n")
	first := true
	for _, col := range t.resultCols {
		v, ok := t.yankBuffer[col]
		if !ok {
			continue
		}
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		keyJSON, _ := json.Marshal(col)
		valJSON, _ := json.Marshal(v)
		buf.WriteString("  ")
		buf.Write(keyJSON)
		buf.WriteString(": ")
		buf.Write(valJSON)
	}
	buf.WriteString("\n}")
	if err := clipboard.Copy(buf.String()); err != nil {
		t.app.ShowWarning(fmt.Sprintf("Yank failed: %v", err))
		return
	}
	t.app.ShowInfo("Row yanked to clipboard")
}

func (t *TableData) yankCell() {
	pos := t.grid.GetCursor()
	value := t.grid.GetCellValue(pos)
	if err := clipboard.Copy(value); err != nil {
		t.app.ShowWarning(fmt.Sprintf("Copy failed: %v", err))
		return
	}
	t.app.ShowInfo(fmt.Sprintf("Copied: %s", truncate(value, 40)))
}

func (t *TableData) pasteRow() {
	if t.yankBuffer == nil {
		t.app.ShowWarning("No row in yank buffer")
		return
	}

	var editableCols []engine.ColumnInfo
	for _, col := range t.columns {
		if !isAutoColumn(col) {
			editableCols = append(editableCols, col)
		}
	}

	// Filter yank buffer to editable columns
	prefill := make(map[string]string)
	for _, col := range editableCols {
		if v, ok := t.yankBuffer[col.Name]; ok {
			prefill[col.Name] = v
		}
	}

	t.showInsertFormWithValues(editableCols, prefill)
}

func (t *TableData) copyRowAsInsert() {
	row := t.grid.GetCursorRow()
	if row == nil {
		t.app.ShowWarning("No row to copy")
		return
	}

	var cols []string
	var vals []string
	for _, colName := range t.resultCols {
		v, ok := row[colName]
		if !ok {
			continue
		}
		cols = append(cols, colName)
		if strings.EqualFold(v, "NULL") || v == "" {
			vals = append(vals, "NULL")
		} else {
			vals = append(vals, "'"+strings.ReplaceAll(v, "'", "''")+"'")
		}
	}

	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
		t.table, strings.Join(cols, ", "), strings.Join(vals, ", "))

	if err := clipboard.Copy(sql); err != nil {
		t.app.ShowError(fmt.Sprintf("Clipboard failed: %v", err))
		return
	}
	t.app.ShowInfo("INSERT copied to clipboard")
}

func (t *TableData) showExportPicker() {
	list := components.NewList().
		SetHighlightFullLine(true).
		SetWrapAround(true)

	list.AddItem("CSV to clipboard")
	list.AddItem("JSON to clipboard")
	list.AddItem("SQL INSERT to clipboard")
	list.AddItem("CSV to file")
	list.AddItem("JSON to file")
	list.AddItem("SQL INSERT to file")

	type exportChoice struct {
		format string
		toFile bool
	}
	choices := []exportChoice{
		{"csv", false}, {"json", false}, {"sql", false},
		{"csv", true}, {"json", true}, {"sql", true},
	}

	list.SetOnSelect(func(index int, _ components.ListItem) {
		t.app.app.Pages().Pop()
		choice := choices[index]
		if choice.toFile {
			t.showExportFilePath(choice.format)
		} else {
			t.exportToClipboard(choice.format)
		}
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Export Data",
		Width:    40,
		Height:   11,
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Select"},
			{Key: "Esc", Description: "Close"},
		})

	t.app.app.Pages().Push(modal)
}

// BuildPipeData implements PipeableView.
func (t *TableData) BuildPipeData(format string) string {
	return t.buildExportData(format)
}

func (t *TableData) buildExportData(format string) string {
	rowCount := t.source.RowCount()
	colCount := t.source.ColCount()

	switch format {
	case "csv":
		var buf strings.Builder
		w := csv.NewWriter(&buf)
		w.Write(t.resultCols)
		for r := 0; r < rowCount; r++ {
			var row []string
			for c := 0; c < colCount; c++ {
				row = append(row, t.source.Cell(r, c).Value)
			}
			w.Write(row)
		}
		w.Flush()
		return buf.String()

	case "json":
		var rows []map[string]string
		for r := 0; r < rowCount; r++ {
			row := make(map[string]string)
			for c := 0; c < colCount; c++ {
				row[t.resultCols[c]] = t.source.Cell(r, c).Value
			}
			rows = append(rows, row)
		}
		data, _ := json.MarshalIndent(rows, "", "  ")
		return string(data)

	case "sql":
		var stmts []string
		for r := 0; r < rowCount; r++ {
			var vals []string
			for c := 0; c < colCount; c++ {
				v := t.source.Cell(r, c).Value
				if strings.EqualFold(v, "NULL") || v == "" {
					vals = append(vals, "NULL")
				} else {
					vals = append(vals, "'"+strings.ReplaceAll(v, "'", "''")+"'")
				}
			}
			stmts = append(stmts, fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);",
				t.table, strings.Join(t.resultCols, ", "), strings.Join(vals, ", ")))
		}
		return strings.Join(stmts, "\n")
	}
	return ""
}

func (t *TableData) exportToClipboard(format string) {
	output := t.buildExportData(format)
	if err := clipboard.Copy(output); err != nil {
		t.app.ShowError(fmt.Sprintf("Clipboard failed: %v", err))
		return
	}
	t.app.ShowSuccess(fmt.Sprintf("Exported %d rows (%d bytes) to clipboard",
		t.source.RowCount(), len(output)))
}

func (t *TableData) showExportFilePath(format string) {
	ext := format
	if format == "sql" {
		ext = "sql"
	}
	defaultPath := fmt.Sprintf("%s.%s", t.table, ext)

	form := components.NewFormBuilder().
		Text("path", "File path").Value(defaultPath).Done().
		OnSubmit(func(values map[string]any) {
			path, _ := values["path"].(string)
			output := t.buildExportData(format)
			if err := os.WriteFile(path, []byte(output), 0644); err != nil {
				t.app.ShowError(fmt.Sprintf("Write failed: %v", err))
			} else {
				t.app.ShowSuccess(fmt.Sprintf("Exported %d rows to %s", t.source.RowCount(), path))
			}
			t.app.app.Pages().Pop()
		}).
		OnCancel(func() {
			t.app.app.Pages().Pop()
		}).
		Build()

	modal := components.NewModal(components.ModalConfig{
		Title:    "Export to File",
		Width:    60,
		Height:   8,
		Backdrop: true,
	}).SetContent(form).
		SetFocusOnShow(form)

	t.app.app.Pages().Push(modal)
}
