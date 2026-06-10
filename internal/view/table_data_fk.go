package view

import (
	"fmt"

	"github.com/atterpac/dado/components"

	"github.com/atterpac/qry/internal/engine"
)

func (t *TableData) followFK() {
	pos := t.grid.GetCursor()
	if pos.Col < 0 || pos.Col >= len(t.resultCols) {
		return
	}

	colName := t.resultCols[pos.Col]
	cellValue := t.grid.GetCellValue(pos)

	// Find FK relationships for this column
	var matches []engine.ForeignKeyInfo
	for _, fk := range t.fkInfo {
		if fk.Column == colName {
			matches = append(matches, fk)
		}
	}

	if len(matches) == 0 {
		t.app.ShowInfo("No foreign key on this column")
		return
	}

	if len(matches) == 1 {
		t.navigateToFK(matches[0], cellValue)
		return
	}

	// Multiple FKs: show picker
	t.showFKPicker(matches, cellValue)
}

func (t *TableData) navigateToFK(fk engine.ForeignKeyInfo, cellValue string) {
	var filter string
	if fk.IsInbound {
		filter = fk.ReferencedColumn + ":" + cellValue
	} else {
		filter = fk.ReferencedColumn + ":" + cellValue
	}
	t.app.NavigateToTableDataWithFilter(fk.ReferencedSchema, fk.ReferencedTable, filter)
}

func (t *TableData) showFKPicker(fks []engine.ForeignKeyInfo, cellValue string) {
	list := components.NewList().
		SetHighlightFullLine(true).
		SetWrapAround(true)

	for _, fk := range fks {
		arrow := "→" // →
		if fk.IsInbound {
			arrow = "←" // ←
		}
		list.AddItem(fmt.Sprintf("%s %s.%s", arrow, fk.ReferencedTable, fk.ReferencedColumn))
	}

	list.SetOnSelect(func(index int, _ components.ListItem) {
		t.app.app.Pages().Pop()
		t.navigateToFK(fks[index], cellValue)
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Follow Foreign Key",
		Width:    50,
		Height:   min(len(fks)+5, 15),
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Select"},
			{Key: "Esc", Description: "Close"},
		})

	t.app.app.Pages().Push(modal)
}
