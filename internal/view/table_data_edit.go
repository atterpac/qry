package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"

	"github.com/atterpac/qry/internal/engine"
)

func (t *TableData) submitChangeset(cs *components.Changeset) {
	hasEdits := cs != nil && cs.HasChanges()
	hasInserts := len(t.pendingInserts) > 0
	hasDeletes := len(t.pendingDeletes) > 0

	if !hasEdits && !hasInserts && !hasDeletes {
		t.app.ShowInfo("No changes to submit")
		return
	}

	if (hasEdits || hasDeletes) && len(t.pkCols) == 0 {
		t.app.ShowError("Cannot submit updates/deletes: table has no primary key")
		return
	}

	provider := t.app.Provider()
	if provider == nil {
		return
	}

	// Build set of rows being deleted (skip updates for these)
	deletedRows := make(map[int]bool)
	for _, pd := range t.pendingDeletes {
		deletedRows[pd.RowIndex] = true
	}

	// Group edit changes by row
	type rowChanges struct {
		row     int
		changes []engine.CellChange
		pkVals  map[string]string
	}

	var previewStatements []string

	// UPDATEs first
	if hasEdits {
		dirtyRows := cs.DirtyRows()
		for rowIdx := range dirtyRows {
			if deletedRows[rowIdx] {
				continue // skip updates for rows being deleted
			}
			rc := rowChanges{row: rowIdx, pkVals: make(map[string]string)}
			for _, pkCol := range t.pkCols {
				for colIdx, colName := range t.resultCols {
					if colName == pkCol {
						cell := t.source.Cell(rowIdx, colIdx)
						rc.pkVals[pkCol] = cell.Value
						break
					}
				}
			}
			var changes []engine.CellChange
			for _, change := range cs.Changes() {
				if change.Position.Row == rowIdx && change.Position.Col < len(t.resultCols) {
					changes = append(changes, engine.CellChange{
						Column:   t.resultCols[change.Position.Col],
						OldValue: change.OldValue,
						NewValue: change.NewValue,
					})
				}
			}
			if len(changes) > 0 {
				sql, args, err := provider.BuildUpdate(t.schema, t.table, t.pkCols, changes, rc.pkVals)
				if err != nil {
					t.app.ShowError(fmt.Sprintf("Build SQL failed: %v", err))
					return
				}
				previewStatements = append(previewStatements, interpolateSQL(sql, args))
			}
		}
	}

	// INSERTs
	for _, pi := range t.pendingInserts {
		sql, args, err := provider.BuildInsert(t.schema, t.table, pi.Columns, pi.Values)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build INSERT failed: %v", err))
			return
		}
		previewStatements = append(previewStatements, interpolateSQL(sql, args))
	}

	// DELETEs last
	for _, pd := range t.pendingDeletes {
		sql, args, err := provider.BuildDelete(t.schema, t.table, t.pkCols, pd.PKValues)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build DELETE failed: %v", err))
			return
		}
		previewStatements = append(previewStatements, interpolateSQL(sql, args))
	}

	t.showConfirmSQL(previewStatements, func() {
		t.executeChanges()
	})
}

// interpolateSQL replaces parameter placeholders ($1, $2, ... or ?) with
// quoted argument values so the confirmation preview shows actual data.
func interpolateSQL(sql string, args []any) string {
	// Replace numbered placeholders ($1, $2, ...) in reverse order so that
	// $10 is replaced before $1.
	result := sql
	for i := len(args) - 1; i >= 0; i-- {
		placeholder := fmt.Sprintf("$%d", i+1)
		result = strings.ReplaceAll(result, placeholder, quoteValue(args[i]))
	}
	// Replace positional ? placeholders (MySQL/SQLite) left-to-right.
	if strings.Contains(result, "?") {
		var b strings.Builder
		argIdx := 0
		for i := 0; i < len(result); i++ {
			if result[i] == '?' && argIdx < len(args) {
				b.WriteString(quoteValue(args[argIdx]))
				argIdx++
			} else {
				b.WriteByte(result[i])
			}
		}
		result = b.String()
	}
	return result
}

func quoteValue(v any) string {
	if v == nil {
		return "NULL"
	}
	return fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", v), "'", "''"))
}

func (t *TableData) showConfirmSQL(statements []string, onConfirm func()) {
	preview := strings.Join(statements, ";\n\n")

	tv := core.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWordWrap(true)
	tv.SetScrollable(true)
	tv.SetText(fmt.Sprintf("[::b]%d statement(s) to execute:[::-]\n\n%s", len(statements), preview))

	content := &confirmContent{
		TextView: tv,
		onConfirm: func() {
			t.app.app.Pages().Pop()
			onConfirm()
		},
	}

	modal := components.NewModal(components.ModalConfig{
		Title:  "Confirm Changes",
		Width:  80,
		Height: min(len(statements)*3+8, 30),
	}).SetContent(content)

	t.app.app.Pages().Push(modal)
}

func (t *TableData) executeChanges() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	cs := t.grid.GetChangeset()

	// Build set of rows being deleted
	deletedRows := make(map[int]bool)
	for _, pd := range t.pendingDeletes {
		deletedRows[pd.RowIndex] = true
	}

	var sqlStmts []string
	var allArgs [][]any

	// UPDATEs
	if cs != nil && cs.HasChanges() {
		dirtyRows := cs.DirtyRows()
		for rowIdx := range dirtyRows {
			if deletedRows[rowIdx] {
				continue
			}
			pkVals := make(map[string]string)
			for _, pkCol := range t.pkCols {
				for colIdx, colName := range t.resultCols {
					if colName == pkCol {
						cell := t.source.Cell(rowIdx, colIdx)
						pkVals[pkCol] = cell.Value
						break
					}
				}
			}
			var changes []engine.CellChange
			for _, change := range cs.Changes() {
				if change.Position.Row == rowIdx && change.Position.Col < len(t.resultCols) {
					changes = append(changes, engine.CellChange{
						Column:   t.resultCols[change.Position.Col],
						OldValue: change.OldValue,
						NewValue: change.NewValue,
					})
				}
			}
			if len(changes) > 0 {
				sql, args, err := provider.BuildUpdate(t.schema, t.table, t.pkCols, changes, pkVals)
				if err != nil {
					t.app.ShowError(fmt.Sprintf("Build SQL failed: %v", err))
					return
				}
				sqlStmts = append(sqlStmts, sql)
				allArgs = append(allArgs, args)
			}
		}
	}

	// INSERTs
	for _, pi := range t.pendingInserts {
		sql, args, err := provider.BuildInsert(t.schema, t.table, pi.Columns, pi.Values)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build INSERT failed: %v", err))
			return
		}
		sqlStmts = append(sqlStmts, sql)
		allArgs = append(allArgs, args)
	}

	// DELETEs last
	for _, pd := range t.pendingDeletes {
		sql, args, err := provider.BuildDelete(t.schema, t.table, t.pkCols, pd.PKValues)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build DELETE failed: %v", err))
			return
		}
		sqlStmts = append(sqlStmts, sql)
		allArgs = append(allArgs, args)
	}

	if len(sqlStmts) == 0 {
		return
	}

	// Execute all statements in a transaction for atomicity.
	// If the user already started one with :begin, use that instead.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Auto-wrap in a transaction if the provider supports it and
		// no user-managed transaction is active.
		autoTx := false
		if tp, ok := provider.(engine.TransactionalProvider); ok && !tp.InTransaction() {
			if err := tp.BeginTx(ctx); err != nil {
				t.app.QueueUpdateDraw(func() {
					t.app.ShowError(fmt.Sprintf("Begin transaction failed: %v", err))
				})
				return
			}
			autoTx = true
		}

		var executed int
		for i, sql := range sqlStmts {
			_, err := provider.ExecuteArgs(ctx, sql, allArgs[i])
			if err != nil {
				// Rollback the auto-transaction on failure
				if autoTx {
					if tp, ok := provider.(engine.TransactionalProvider); ok {
						tp.RollbackTx(ctx)
					}
				}
				t.app.QueueUpdateDraw(func() {
					t.app.ShowError(fmt.Sprintf("Execute failed (%d/%d), rolled back: %v", i+1, len(sqlStmts), err))
				})
				return
			}
			executed++
		}

		// Auto-commit if we started the transaction ourselves.
		// If the user started it with :begin, leave it open for them to :commit/:rollback.
		if autoTx {
			if tp, ok := provider.(engine.TransactionalProvider); ok {
				if err := tp.CommitTx(ctx); err != nil {
					t.app.QueueUpdateDraw(func() {
						t.app.ShowError(fmt.Sprintf("Commit failed: %v", err))
					})
					return
				}
			}
		}

		t.app.QueueUpdateDraw(func() {
			if cs != nil {
				cs.Clear()
			}
			t.pendingInserts = nil
			t.pendingDeletes = nil
			t.app.ShowSuccess(fmt.Sprintf("Executed %d statement(s)", executed))
			t.loadData()
		})
	}()
}

// dryRun executes pending changes inside a BEGIN/ROLLBACK to preview effects
// without persisting them.
func (t *TableData) dryRun() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	cs := t.grid.GetChangeset()
	hasEdits := cs != nil && cs.HasChanges()
	hasInserts := len(t.pendingInserts) > 0
	hasDeletes := len(t.pendingDeletes) > 0

	if !hasEdits && !hasInserts && !hasDeletes {
		t.app.ShowInfo("No changes for dry-run")
		return
	}

	tp, ok := provider.(engine.TransactionalProvider)
	if !ok {
		t.app.ShowWarning("Current engine does not support dry-run")
		return
	}

	if tp.InTransaction() {
		t.app.ShowWarning("Cannot dry-run while a transaction is active")
		return
	}

	// Build statements (reuse submitChangeset logic for preview)
	deletedRows := make(map[int]bool)
	for _, pd := range t.pendingDeletes {
		deletedRows[pd.RowIndex] = true
	}

	var previewStatements []string
	if hasEdits {
		dirtyRows := cs.DirtyRows()
		for rowIdx := range dirtyRows {
			if deletedRows[rowIdx] {
				continue
			}
			pkVals := make(map[string]string)
			for _, pkCol := range t.pkCols {
				for colIdx, colName := range t.resultCols {
					if colName == pkCol {
						cell := t.source.Cell(rowIdx, colIdx)
						pkVals[pkCol] = cell.Value
						break
					}
				}
			}
			var changes []engine.CellChange
			for _, change := range cs.Changes() {
				if change.Position.Row == rowIdx && change.Position.Col < len(t.resultCols) {
					changes = append(changes, engine.CellChange{
						Column:   t.resultCols[change.Position.Col],
						OldValue: change.OldValue,
						NewValue: change.NewValue,
					})
				}
			}
			if len(changes) > 0 {
				sql, args, err := provider.BuildUpdate(t.schema, t.table, t.pkCols, changes, pkVals)
				if err != nil {
					t.app.ShowError(fmt.Sprintf("Build SQL failed: %v", err))
					return
				}
				previewStatements = append(previewStatements, interpolateSQL(sql, args))
			}
		}
	}
	for _, pi := range t.pendingInserts {
		sql, args, err := provider.BuildInsert(t.schema, t.table, pi.Columns, pi.Values)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build INSERT failed: %v", err))
			return
		}
		previewStatements = append(previewStatements, interpolateSQL(sql, args))
	}
	for _, pd := range t.pendingDeletes {
		sql, args, err := provider.BuildDelete(t.schema, t.table, t.pkCols, pd.PKValues)
		if err != nil {
			t.app.ShowError(fmt.Sprintf("Build DELETE failed: %v", err))
			return
		}
		previewStatements = append(previewStatements, interpolateSQL(sql, args))
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := tp.BeginTx(ctx); err != nil {
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Dry-run begin failed: %v", err))
			})
			return
		}

		var results []string
		for _, stmt := range previewStatements {
			result, err := provider.ExecuteQuery(ctx, stmt)
			if err != nil {
				results = append(results, fmt.Sprintf("ERROR: %v", err))
			} else if result.Message != "" {
				results = append(results, result.Message)
			} else {
				results = append(results, "OK")
			}
		}

		tp.RollbackTx(ctx)

		t.app.QueueUpdateDraw(func() {
			summary := fmt.Sprintf("Dry-run results (%d statements, all rolled back):\n\n", len(previewStatements))
			for i, r := range results {
				summary += fmt.Sprintf("%d. %s → %s\n", i+1, previewStatements[i], r)
			}
			t.app.ShowInfo("Dry-run complete — all changes rolled back")

			tv := core.NewTextView()
			tv.SetDynamicColors(true)
			tv.SetWordWrap(true)
			tv.SetScrollable(true)
			tv.SetText(summary)

			modal := components.NewModal(components.ModalConfig{
				Title:  "Dry-Run Results",
				Width:  80,
				Height: min(len(previewStatements)*2+8, 30),
			}).SetContent(tv)

			t.app.app.Pages().Push(modal)
		})
	}()
}

// discardChanges clears all pending edits, inserts, and deletes.
func (t *TableData) discardChanges() {
	cs := t.grid.GetChangeset()
	hadChanges := false

	if cs != nil && cs.HasChanges() {
		cs.Clear()
		hadChanges = true
	}
	if len(t.pendingInserts) > 0 {
		t.pendingInserts = nil
		hadChanges = true
	}
	if len(t.pendingDeletes) > 0 {
		t.pendingDeletes = nil
		hadChanges = true
	}

	if !hadChanges {
		t.app.ShowInfo("No pending changes to discard")
		return
	}

	t.updateStatusBar()
	t.loadData()
	t.app.ShowSuccess("All pending changes discarded")
}

// isAutoColumn returns true if the column is auto-generated (auto_increment, serial, etc).
func isAutoColumn(col engine.ColumnInfo) bool {
	extra := strings.ToLower(col.Extra)
	def := strings.ToLower(col.Default)
	return strings.Contains(extra, "auto_increment") ||
		strings.Contains(extra, "generated") ||
		strings.Contains(def, "nextval") ||
		strings.Contains(def, "gen_random")
}

func (t *TableData) deleteRows() {
	if len(t.pkCols) == 0 {
		t.app.ShowError("Cannot delete: no primary key")
		return
	}

	selected := t.grid.GetSelectedRowIndices()
	if len(selected) == 0 {
		selected = []int{t.grid.GetCursorRowIndex()}
	}

	for _, rowIdx := range selected {
		pkVals := make(map[string]string)
		for _, pkCol := range t.pkCols {
			for colIdx, colName := range t.resultCols {
				if colName == pkCol {
					cell := t.source.Cell(rowIdx, colIdx)
					pkVals[pkCol] = cell.Value
					break
				}
			}
		}
		t.pendingDeletes = append(t.pendingDeletes, PendingDelete{
			RowIndex: rowIdx,
			PKValues: pkVals,
		})
	}

	t.grid.ClearRowSelection()
	t.applyDeletionMarks()
	t.updateStatusBar()
	t.app.ShowInfo(fmt.Sprintf("%d row(s) staged for deletion", len(selected)))
}

// applyDeletionMarks syncs the grid's deletion highlight with t.pendingDeletes.
// Called after staging deletes and after every reload (row indices are only
// valid for the currently loaded page).
func (t *TableData) applyDeletionMarks() {
	t.grid.ClearDeletedRows()
	for _, pd := range t.pendingDeletes {
		t.grid.SetRowDeleted(pd.RowIndex, true)
	}
}

func (t *TableData) showInsertForm() {
	// Filter to editable columns
	var editableCols []engine.ColumnInfo
	for _, col := range t.columns {
		if !isAutoColumn(col) {
			editableCols = append(editableCols, col)
		}
	}

	if len(editableCols) == 0 {
		t.app.ShowWarning("No editable columns found")
		return
	}

	t.showInsertFormWithValues(editableCols, nil)
}

func (t *TableData) showInsertFormWithValues(editableCols []engine.ColumnInfo, prefill map[string]string) {
	fb := components.NewFormBuilder()
	for _, col := range editableCols {
		defaultVal := ""
		if prefill != nil {
			if v, ok := prefill[col.Name]; ok {
				defaultVal = v
			}
		} else if col.Default != "" && !strings.HasPrefix(strings.ToLower(col.Default), "null") {
			defaultVal = col.Default
		}
		label := col.Name
		if !col.Nullable {
			label += " *"
		}
		fb = fb.Text(col.Name, label).Value(defaultVal).Done()
	}

	form := fb.OnSubmit(func(values map[string]any) {
		var cols []string
		var vals []string
		for _, col := range editableCols {
			val, _ := values[col.Name].(string)
			if val != "" {
				cols = append(cols, col.Name)
				vals = append(vals, val)
			} else if !col.Nullable {
				t.app.ShowWarning(fmt.Sprintf("Column %s is required", col.Name))
				return
			}
		}
		t.pendingInserts = append(t.pendingInserts, PendingInsert{
			Columns: cols,
			Values:  vals,
		})
		t.app.app.Pages().Pop()
		t.updateStatusBar()
		t.app.ShowInfo("Row staged for insert")
	}).OnCancel(func() {
		t.app.app.Pages().Pop()
	}).Build()

	modal := components.NewModal(components.ModalConfig{
		Title:    "Insert Row",
		Width:    60,
		Height:   min(len(editableCols)*2+8, 30),
		Backdrop: true,
	}).SetContent(form).
		SetFocusOnShow(form).
		SetHints([]components.KeyHint{
			{Key: "Tab", Description: "Next field"},
			{Key: "Enter", Description: "Submit"},
			{Key: "Esc", Description: "Cancel"},
		})

	t.app.app.Pages().Push(modal)
}

func (t *TableData) repeatLastEdit() {
	if !t.hasLastEdit {
		t.app.ShowInfo("No previous edit to repeat")
		return
	}

	cursorRow := t.grid.GetCursorRowIndex()
	pos := components.CellPosition{Row: cursorRow, Col: t.lastEditCol}
	oldVal := t.grid.GetCellValue(pos)
	t.grid.GetChangeset().RecordChange(pos, oldVal, t.lastEditValue)
	t.updateStatusBar()
}
