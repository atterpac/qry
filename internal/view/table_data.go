package view

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/atterpac/qry/internal/clipboard"
	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
)

// PendingDelete represents a row staged for deletion.
type PendingDelete struct {
	RowIndex int
	PKValues map[string]string
}

// PendingInsert represents a row staged for insertion.
type PendingInsert struct {
	Columns []string
	Values  []string
}

// TableData shows a DataGrid with table contents and inline editing support.
type TableData struct {
	*tview.Flex
	app       *App
	schema    string
	table     string
	grid      *components.DataGrid
	source    *components.SliceSource
	columns   []engine.ColumnInfo
	pkCols    []string
	resultCols   []string
	offset       int
	limit        int
	searchFilter string
	searchActive bool
	statusBar    *tview.TextView
	emptyState     *tview.TextView
	fkInfo         []engine.ForeignKeyInfo
	gPressed       bool
	searchCancel   context.CancelFunc

	// Pending mutations
	pendingDeletes []PendingDelete
	pendingInserts []PendingInsert

	// Yank buffer
	yankBuffer map[string]string

	// Dot-repeat state
	lastEditCol   int
	lastEditValue string
	hasLastEdit   bool

	// Key sequence trackers
	dPressed  bool
	yPressed  bool
	yankTimer *time.Timer

	// Sort state
	sortColumn string
	sortDir    string

	// Detail panel (row JSON view)
	detailPanel   *components.Panel
	detailText    *tview.TextView
	detailVisible bool
	gridFlex      *tview.Flex
}

func NewTableData(app *App, schema, table string) *TableData {
	flex := tview.NewFlex()
	theme.Register(flex)
	statusBar := tview.NewTextView()
	theme.Register(statusBar)
	t := &TableData{
		Flex:      flex,
		app:       app,
		schema:    schema,
		table:     table,
		limit:     100,
		source:    components.NewSliceSource(nil, nil),
		statusBar: statusBar,
	}

	t.grid = components.NewDataGrid()
	t.grid.SetShowRowNumbers(true)
	t.grid.SetShowHeader(true)
	t.grid.SetSource(t.source)

	// Status bar for changeset info
	t.statusBar.SetDynamicColors(true)
	t.statusBar.SetTextAlign(tview.AlignLeft)
	t.updateStatusBar()

	t.grid.SetOnModeChange(func(mode components.GridMode) {
		app.gridEditing = mode == components.GridModeEdit
	})

	t.grid.SetOnBack(func() {
		// Let global handler deal with Esc
	})

	t.grid.SetOnCopy(func(value string) {
		if err := clipboard.Copy(value); err != nil {
			app.ShowWarning(fmt.Sprintf("Copy failed: %v", err))
			return
		}
		app.ShowInfo(fmt.Sprintf("Copied: %s", truncate(value, 40)))
	})

	t.grid.SetOnChangesetUpdate(func(cs *components.Changeset) {
		t.updateStatusBar()
	})

	t.grid.SetOnSearch(func() {
		t.showSearchBar()
	})

	// Capture cell edits for dot-repeat
	t.grid.SetOnCellEdit(func(pos components.CellPosition, oldValue, newValue string) {
		t.lastEditCol = pos.Col
		t.lastEditValue = newValue
		t.hasLastEdit = true
	})

	// Ctrl+S submits changeset
	t.grid.SetOnSubmit(func(cs *components.Changeset) {
		t.submitChangeset(cs)
	})

	// Detail panel for row JSON view
	t.detailText = tview.NewTextView()
	t.detailText.SetDynamicColors(true)
	t.detailText.SetScrollable(true)
	t.detailText.SetWordWrap(true)
	theme.Register(t.detailText)
	t.detailPanel = components.NewPanel().SetTitle("Row Detail").SetContent(t.detailText)

	// Inner flex to hold grid + statusBar as a row group
	t.gridFlex = tview.NewFlex()
	t.gridFlex.SetDirection(tview.FlexRow)
	theme.Register(t.gridFlex)

	t.grid.SetOnCursorMove(func(pos components.CellPosition) {
		if t.detailVisible {
			// Must run in a goroutine: onCursorMove fires inside the DataGrid
			// input handler on the event loop. QueueUpdateDraw blocks until f
			// executes, but f can't run until the current handler returns — so
			// calling it synchronously deadlocks. The goroutine lets the input
			// handler finish and release both the event loop and dg.mu, after
			// which QueueUpdateDraw can acquire them to run updateDetailPanel.
			go t.app.QueueUpdateDraw(func() {
				t.updateDetailPanel()
			})
		}
	})

	t.grid.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape && t.searchActive {
			if t.searchCancel != nil {
				t.searchCancel()
			}
			t.app.statusBar.ExitCommandMode()
			t.searchFilter = ""
			t.searchActive = false
			app.gridSearching = false
			t.offset = 0
			t.loadData()
			return nil
		}

		// Handle gd sequence for FK traversal
		if t.gPressed {
			t.gPressed = false
			if event.Rune() == 'd' {
				t.followFK()
				return nil
			}
			return event
		}

		// Handle dd sequence for delete
		if t.dPressed {
			t.dPressed = false
			if event.Rune() == 'd' {
				t.deleteRows()
				return nil
			}
			return event
		}

		// Handle yy/yp sequences
		if t.yPressed {
			t.yPressed = false
			if t.yankTimer != nil {
				t.yankTimer.Stop()
				t.yankTimer = nil
			}
			switch event.Rune() {
			case 'y':
				t.yankRow()
				return nil
			case 'p':
				t.pasteRow()
				return nil
			}
			return event
		}

		if event.Rune() == 'g' {
			t.gPressed = true
			return event // let DataGrid handle gg
		}

		// Ctrl key combinations
		switch event.Key() {
		case tcell.KeyCtrlY:
			t.copyRowAsInsert()
			return nil
		case tcell.KeyCtrlE:
			t.showExportPicker()
			return nil
		}

		switch event.Rune() {
		case 'n':
			t.nextPage()
			return nil
		case 'N':
			t.prevPage()
			return nil
		case 'd':
			t.dPressed = true
			return nil
		case 'y':
			t.yPressed = true
			t.yankTimer = time.AfterFunc(200*time.Millisecond, func() {
				app.QueueUpdateDraw(func() {
					if t.yPressed {
						t.yPressed = false
						t.yankCell()
					}
				})
			})
			return nil
		case 'o', 'O':
			t.showInsertForm()
			return nil
		case '.':
			t.repeatLastEdit()
			return nil
		case 'p':
			t.toggleDetailPanel()
			return nil
		case 'W':
			t.showCellDetail()
			return nil
		case 'm':
			t.saveBookmark()
			return nil
		case '\'':
			t.showBookmarkPicker()
			return nil
		}
		return event
	})

	t.emptyState = tview.NewTextView()
	t.emptyState.SetDynamicColors(true)
	t.emptyState.SetTextAlign(tview.AlignCenter)
	t.emptyState.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Pass all keys through so global handlers (q, Esc, P, etc.) work
		return event
	})
	theme.Register(t.emptyState)

	// Initial layout (will be rebuilt by setGridData / rebuildLayout)
	t.rebuildLayout(false)

	return t
}

func (t *TableData) Name() string { return t.table }

func (t *TableData) Start() {
	t.loadData()
}

func (t *TableData) Stop() {
	t.app.gridSearching = false
}

func (t *TableData) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "h/l", Description: "Navigate columns"},
		{Key: "j/k", Description: "Navigate rows"},
		{Key: "Enter", Description: "Edit cell"},
		{Key: "n/N", Description: "Next/prev page"},
		{Key: "/", Description: "Search/filter"},
		{Key: "gd", Description: "Follow FK"},
		{Key: "dd", Description: "Delete row"},
		{Key: "o", Description: "Insert row"},
		{Key: "yy/yp", Description: "Yank/paste row"},
		{Key: ".", Description: "Repeat edit"},
		{Key: "p", Description: "Row detail"},
		{Key: "W", Description: "Cell detail"},
		{Key: "Space", Description: "Toggle select"},
		{Key: "m", Description: "Bookmark"},
		{Key: "'", Description: "Jump to bookmark"},
		{Key: "Ctrl+Y", Description: "Copy as INSERT"},
		{Key: "Ctrl+E", Description: "Export"},
		{Key: "Ctrl+S", Description: "Submit changes"},
		{Key: "u", Description: "Undo cell"},
	}
}

func (t *TableData) updateStatusBar() {
	cs := t.grid.GetChangeset()
	editCount := 0
	if cs != nil {
		editCount = cs.Count()
	}
	insertCount := len(t.pendingInserts)
	deleteCount := len(t.pendingDeletes)
	hasPending := editCount > 0 || insertCount > 0 || deleteCount > 0

	if !hasPending {
		page := t.offset/t.limit + 1
		filterInfo := ""
		if t.searchActive {
			filterInfo = fmt.Sprintf(" · [yellow]filter: %s[-]", t.searchFilter)
		}
		sortInfo := ""
		if t.sortColumn != "" {
			sortInfo = fmt.Sprintf(" · [%s]sort: %s %s[-]", theme.TagAccent(), t.sortColumn, t.sortDir)
		}
		t.statusBar.SetText(fmt.Sprintf(" [%s]Page %d · %d rows · LIMIT %d[-]%s%s",
			theme.TagFgDim(), page, t.source.RowCount(), t.limit, filterInfo, sortInfo))
		return
	}

	var parts []string
	if editCount > 0 {
		parts = append(parts, fmt.Sprintf("%d edit(s)", editCount))
	}
	if insertCount > 0 {
		parts = append(parts, fmt.Sprintf("%d insert(s)", insertCount))
	}
	if deleteCount > 0 {
		parts = append(parts, fmt.Sprintf("%d delete(s)", deleteCount))
	}
	t.statusBar.SetText(fmt.Sprintf(" [yellow]%s[-] · [%s]Ctrl+S to submit[-]",
		strings.Join(parts, " · "), theme.TagFgDim()))
}

func (t *TableData) loadData() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Get column info for PK detection
		columns, err := provider.DescribeTable(ctx, t.schema, t.table)
		if err != nil {
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Describe table failed: %v", err))
			})
			return
		}

		// Extract PK columns
		var pkCols []string
		for _, col := range columns {
			if col.IsPrimaryKey {
				pkCols = append(pkCols, col.Name)
			}
		}

		// Build SELECT query
		tableName := provider.QuoteIdentifier(t.table)
		if t.schema != "" {
			caps := provider.Capabilities()
			if caps.HasSchemas {
				tableName = provider.QuoteIdentifier(t.schema) + "." + tableName
			}
		}

		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		if t.searchActive && t.searchFilter != "" {
			var colNames []string
			for _, c := range columns {
				colNames = append(colNames, c.Name)
			}
			filters := engine.ParseSearchInput(t.searchFilter, colNames)
			if clause := provider.BuildSearchClause(colNames, filters); clause != "" {
				query += " WHERE " + clause
			}
		}
		if t.sortColumn != "" {
			query += fmt.Sprintf(" ORDER BY %s %s", provider.QuoteIdentifier(t.sortColumn), t.sortDir)
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", t.limit, t.offset)
		result, err := provider.ExecuteQuery(ctx, query)
		if err != nil {
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Query failed: %v", err))
			})
			return
		}

		// Fetch FK info (non-critical, ignore errors)
		fkInfo, _ := provider.GetForeignKeys(ctx, t.schema, t.table)

		t.app.QueueUpdateDraw(func() {
			t.columns = columns
			t.pkCols = pkCols
			t.fkInfo = fkInfo
			t.setGridData(result)
		})
	}()
}

func (t *TableData) setGridData(result *engine.QueryResult) {
	if result == nil || len(result.Columns) == 0 {
		return
	}
	t.resultCols = result.Columns
	t.source.SetSliceData(result.Columns, result.Rows)

	showEmpty := result.RowCount == 0
	if showEmpty {
		msg := fmt.Sprintf("\n\n[%s]No records found[-]", theme.TagFgDim())
		if t.searchActive {
			msg = fmt.Sprintf("\n\n[%s]No matching records for[-] [%s]%s[-]",
				theme.TagFgDim(), theme.TagAccent(), t.searchFilter)
		}
		t.emptyState.SetText(msg)
	}
	t.rebuildLayout(showEmpty)
	if showEmpty {
		t.app.app.SetFocus(t.emptyState)
	} else {
		t.app.app.SetFocus(t.grid)
	}
	t.updateStatusBar()
	if t.detailVisible && !showEmpty {
		t.updateDetailPanel()
	}
}

func (t *TableData) nextPage() {
	t.offset += t.limit
	t.loadData()
}

func (t *TableData) prevPage() {
	t.offset -= t.limit
	if t.offset < 0 {
		t.offset = 0
	}
	t.loadData()
}

// searchData runs a filtered query reusing cached column info.
// It cancels any previous in-flight search query.
func (t *TableData) searchData() {
	provider := t.app.Provider()
	if provider == nil || len(t.columns) == 0 {
		return
	}

	// Cancel any in-flight search
	if t.searchCancel != nil {
		t.searchCancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.searchCancel = cancel

	go func() {
		defer cancel()

		tableName := provider.QuoteIdentifier(t.table)
		if t.schema != "" {
			caps := provider.Capabilities()
			if caps.HasSchemas {
				tableName = provider.QuoteIdentifier(t.schema) + "." + tableName
			}
		}

		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		if t.searchActive && t.searchFilter != "" {
			var colNames []string
			for _, c := range t.columns {
				colNames = append(colNames, c.Name)
			}
			filters := engine.ParseSearchInput(t.searchFilter, colNames)
			if clause := provider.BuildSearchClause(colNames, filters); clause != "" {
				query += " WHERE " + clause
			}
		}
		if t.sortColumn != "" {
			query += fmt.Sprintf(" ORDER BY %s %s", provider.QuoteIdentifier(t.sortColumn), t.sortDir)
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", t.limit, t.offset)

		result, err := provider.ExecuteQuery(ctx, query)
		if err != nil {
			if ctx.Err() != nil {
				return // cancelled, ignore
			}
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Query failed: %v", err))
			})
			return
		}

		t.app.QueueUpdateDraw(func() {
			t.setGridData(result)
		})
	}()
}

func (t *TableData) showSearchBar() {
	t.app.statusBar.SetCommandPrompt("/ ")
	t.app.statusBar.SetCommandPlaceholder("search... (col:value or term)")

	var colNames []string
	for _, c := range t.columns {
		colNames = append(colNames, c.Name)
	}
	t.app.statusBar.SetOnComplete(func(input string) []string {
		// Offer column name completions with : suffix
		var matches []string
		// Get the last token being typed
		parts := strings.Fields(input)
		var prefix string
		if len(parts) > 0 {
			prefix = parts[len(parts)-1]
		}
		for _, col := range colNames {
			candidate := col + ":"
			if prefix == "" || strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
				matches = append(matches, candidate)
			}
		}
		return matches
	})

	t.app.statusBar.EnterCommandMode()
	t.app.app.SetFocus(t.app.statusBar.GetCommandInput())

	t.app.statusBar.SetOnCommandSubmit(func(text string) {
		t.app.statusBar.ExitCommandMode()
		text = strings.TrimSpace(text)
		if text != "" {
			t.searchFilter = text
			t.searchActive = true
			t.app.gridSearching = true
			t.offset = 0
			t.searchData()
		} else {
			t.searchFilter = ""
			t.searchActive = false
			t.app.gridSearching = false
			t.offset = 0
			t.loadData()
		}
		t.app.refocusCurrent()
	})
	t.app.statusBar.SetOnCommandCancel(func() {
		if t.searchCancel != nil {
			t.searchCancel()
		}
		t.app.statusBar.ExitCommandMode()
		// Restore to unfiltered state
		t.searchFilter = ""
		t.searchActive = false
		t.app.gridSearching = false
		t.offset = 0
		t.loadData()
		t.app.refocusCurrent()
	})
}

// SetFilter configures a pre-applied search filter for this view.
func (t *TableData) SetFilter(filter string) {
	t.searchFilter = filter
	t.searchActive = true
	t.app.gridSearching = true
}

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
		arrow := "\u2192" // →
		if fk.IsInbound {
			arrow = "\u2190" // ←
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
		Height:  min(len(fks)+5, 15),
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Select"},
			{Key: "Esc", Description: "Close"},
		})

	t.app.app.Pages().Push(modal)
}

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

	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetWordWrap(true)
	tv.SetText(fmt.Sprintf("[::b]%d statement(s) to execute:[::-]\n\n%s", len(statements), preview))

	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter || event.Rune() == 'y' {
			t.app.app.Pages().Pop()
			onConfirm()
			return nil
		}
		return event
	})

	modal := components.NewModal(components.ModalConfig{
		Title:  "Confirm Changes",
		Width:  80,
		Height: min(len(statements)*3+8, 30),
	}).SetContent(tv)

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

	// Execute each statement
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var executed int
		for i, sql := range sqlStmts {
			_, err := provider.ExecuteArgs(ctx, sql, allArgs[i])
			if err != nil {
				t.app.QueueUpdateDraw(func() {
					t.app.ShowError(fmt.Sprintf("Execute failed (%d/%d): %v", i+1, len(sqlStmts), err))
				})
				return
			}
			executed++
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

// isAutoColumn returns true if the column is auto-generated (auto_increment, serial, etc).
func isAutoColumn(col engine.ColumnInfo) bool {
	extra := strings.ToLower(col.Extra)
	def := strings.ToLower(col.Default)
	return strings.Contains(extra, "auto_increment") ||
		strings.Contains(extra, "generated") ||
		strings.Contains(def, "nextval") ||
		strings.Contains(def, "gen_random")
}

// --- Phase 1: Delete / Insert ---

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
	t.updateStatusBar()
	t.app.ShowInfo(fmt.Sprintf("%d row(s) staged for deletion", len(selected)))
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
	form := tview.NewForm()
	theme.Register(form)

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
		form.AddInputField(label, defaultVal, 40, nil, nil)
	}

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			t.app.app.Pages().Pop()
			return nil
		}
		return event
	})

	form.AddButton("Submit", func() {
		var cols []string
		var vals []string
		for i, col := range editableCols {
			item := form.GetFormItem(i)
			if input, ok := item.(*tview.InputField); ok {
				val := input.GetText()
				if val != "" {
					cols = append(cols, col.Name)
					vals = append(vals, val)
				} else if !col.Nullable {
					t.app.ShowWarning(fmt.Sprintf("Column %s is required", col.Name))
					return
				}
			}
		}
		t.pendingInserts = append(t.pendingInserts, PendingInsert{
			Columns: cols,
			Values:  vals,
		})
		t.app.app.Pages().Pop()
		t.updateStatusBar()
		t.app.ShowInfo("Row staged for insert")
	})

	form.AddButton("Cancel", func() {
		t.app.app.Pages().Pop()
	})

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

// --- Phase 2: Yank / Paste / Export ---

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
	form := tview.NewForm()
	theme.Register(form)

	ext := format
	if format == "sql" {
		ext = "sql"
	}
	defaultPath := fmt.Sprintf("%s.%s", t.table, ext)

	form.AddInputField("File path", defaultPath, 50, nil, nil)
	form.AddButton("Export", func() {
		path := form.GetFormItem(0).(*tview.InputField).GetText()
		output := t.buildExportData(format)
		if err := os.WriteFile(path, []byte(output), 0644); err != nil {
			t.app.ShowError(fmt.Sprintf("Write failed: %v", err))
		} else {
			t.app.ShowSuccess(fmt.Sprintf("Exported %d rows to %s", t.source.RowCount(), path))
		}
		t.app.app.Pages().Pop()
	})
	form.AddButton("Cancel", func() {
		t.app.app.Pages().Pop()
	})

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			t.app.app.Pages().Pop()
			return nil
		}
		return event
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Export to File",
		Width:    60,
		Height:   8,
		Backdrop: true,
	}).SetContent(form).
		SetFocusOnShow(form)

	t.app.app.Pages().Push(modal)
}

// --- Phase 3: Dot-repeat / Cell Detail ---

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

func (t *TableData) showCellDetail() {
	pos := t.grid.GetCursor()
	if pos.Col < 0 || pos.Col >= len(t.resultCols) {
		return
	}

	colName := t.resultCols[pos.Col]
	value := t.grid.GetCellValue(pos)

	tv := tview.NewTextView()
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

// --- Phase 4: Sort / Count / Schema (called from command bar) ---

func (t *TableData) SetSort(col, dir string) {
	t.sortColumn = col
	t.sortDir = dir
	t.offset = 0
	t.loadData()
}

func (t *TableData) runCount() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tableName := provider.QuoteIdentifier(t.table)
		if t.schema != "" {
			caps := provider.Capabilities()
			if caps.HasSchemas {
				tableName = provider.QuoteIdentifier(t.schema) + "." + tableName
			}
		}

		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
		if t.searchActive && t.searchFilter != "" {
			var colNames []string
			for _, c := range t.columns {
				colNames = append(colNames, c.Name)
			}
			filters := engine.ParseSearchInput(t.searchFilter, colNames)
			if clause := provider.BuildSearchClause(colNames, filters); clause != "" {
				query += " WHERE " + clause
			}
		}

		result, err := provider.ExecuteQuery(ctx, query)
		if err != nil {
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Count failed: %v", err))
			})
			return
		}

		count := "?"
		if result != nil && len(result.Rows) > 0 && len(result.Rows[0]) > 0 {
			count = result.Rows[0][0]
		}

		t.app.QueueUpdateDraw(func() {
			t.app.ShowInfo(fmt.Sprintf("Total rows: %s", count))
		})
	}()
}

func (t *TableData) showSchemaOverlay() {
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("[::b]Schema: %s[::-]\n\n", t.table))

	for _, col := range t.columns {
		tags := ""
		if col.IsPrimaryKey {
			tags += " [yellow]PK[-]"
		}
		if !col.Nullable {
			tags += " [red]NOT NULL[-]"
		}
		if col.Default != "" {
			tags += fmt.Sprintf(" [%s]DEFAULT %s[-]", theme.TagFgDim(), col.Default)
		}
		if col.Extra != "" {
			tags += fmt.Sprintf(" [%s]%s[-]", theme.TagFgDim(), col.Extra)
		}
		buf.WriteString(fmt.Sprintf("  [::b]%s[::-]  %s%s\n", col.Name, col.DataType, tags))
	}

	tv := tview.NewTextView()
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

// --- Phase 5: Bookmarks ---

func (t *TableData) saveBookmark() {
	form := tview.NewForm()
	theme.Register(form)

	defaultName := t.table
	if t.schema != "" {
		defaultName = t.schema + "." + t.table
	}

	form.AddInputField("Bookmark name", defaultName, 40, nil, nil)
	form.AddButton("Save", func() {
		name := form.GetFormItem(0).(*tview.InputField).GetText()
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
	})
	form.AddButton("Cancel", func() {
		t.app.app.Pages().Pop()
	})

	form.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			t.app.app.Pages().Pop()
			return nil
		}
		return event
	})

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

// --- Row Detail Panel ---

func (t *TableData) rebuildLayout(showEmpty bool) {
	t.Clear()
	t.gridFlex.Clear()

	if t.detailVisible {
		// Outer flex is columns: [gridFlex | detailPanel]
		t.SetDirection(tview.FlexColumn)

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
		t.SetDirection(tview.FlexRow)

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
	if !showEmpty {
		t.app.app.SetFocus(t.grid)
	}
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
		buf.WriteString(fmt.Sprintf("  [%s]%s[-]: ", accent, string(keyJSON)))
		formatDetailValue(&buf, v, accent, "  ")
	}
	buf.WriteString("\n}")
	t.detailText.SetText(buf.String())
	t.detailText.ScrollToBeginning()
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
			buf.WriteString(fmt.Sprintf("[%s]%s[-]: ", accent, string(keyJSON)))
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

// CommandContext implements CommandContextProvider.
func (t *TableData) CommandContext() CommandViewContext {
	ctx := CommandViewContext{
		Schema: t.schema,
		Table:  t.table,
	}
	if t.app.Provider() != nil {
		ctx.Engine = string(t.app.Provider().EngineType())
	}
	return ctx
}

var _ nav.Component = (*TableData)(nil)
