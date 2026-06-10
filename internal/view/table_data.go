package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/dado/clipboard"
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
	*core.Flex
	app           *App
	schema        string
	table         string
	grid          *components.DataGrid
	source        *components.SliceSource
	columns       []engine.ColumnInfo
	pkCols        []string
	resultCols    []string
	offset        int
	limit         int
	searchFilter  string
	searchActive  bool
	filterFromNav bool // true when filter was set by FK navigation (gd), not user search
	statusBar     *core.TextView
	emptyState    *core.TextView
	fkInfo        []engine.ForeignKeyInfo
	gPressed      bool
	searchCancel  context.CancelFunc

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
	detailText    *core.TextView
	detailVisible bool
	gridFlex      *core.Flex
}

func NewTableData(app *App, schema, table string) *TableData {
	flex := core.NewFlex()
	statusBar := core.NewTextView()
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
	t.statusBar.SetTextAlign(core.AlignLeft)
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
	t.detailText = core.NewTextView()
	t.detailText.SetDynamicColors(true)
	t.detailText.SetScrollable(true)
	t.detailText.SetWordWrap(true)
	t.detailPanel = components.NewPanel().SetTitle("Row Detail").SetContent(t.detailText)

	// Inner flex to hold grid + statusBar as a row group
	t.gridFlex = core.NewFlex()
	t.gridFlex.SetDirection(core.Column)

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

	t.emptyState = core.NewTextView()
	t.emptyState.SetDynamicColors(true)
	t.emptyState.SetTextAlign(core.AlignCenter)

	// Initial layout (will be rebuilt by setGridData / rebuildLayout)
	t.rebuildLayout(false)

	return t
}

// HandleKey implements the page-level key routing for the table view. It
// replaces the old t.grid.SetInputCapture: where the capture returned nil
// (consumed) we return true; where it returned the event (not consumed) we
// fall through to the DataGrid's own HandleKey.
func (t *TableData) HandleKey(event *tcell.EventKey) bool {
	app := t.app

	// When grid is in edit mode, skip all table keybinds so the DataGrid
	// can handle text input, Escape (cancel), and Enter (confirm).
	if app.gridEditing {
		return t.grid.HandleKey(event)
	}

	if event.Key() == tcell.KeyEscape && t.searchActive && !t.filterFromNav {
		if t.searchCancel != nil {
			t.searchCancel()
		}
		t.app.statusBar.ExitCommandMode()
		t.searchFilter = ""
		t.searchActive = false
		app.gridSearching = false
		t.offset = 0
		t.loadData()
		return true
	}

	// Handle gd sequence for FK traversal
	if t.gPressed {
		t.gPressed = false
		if event.Rune() == 'd' {
			t.followFK()
			return true
		}
		return t.grid.HandleKey(event)
	}

	// Handle dd sequence for delete
	if t.dPressed {
		t.dPressed = false
		if event.Rune() == 'd' {
			t.deleteRows()
			return true
		}
		return t.grid.HandleKey(event)
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
			return true
		case 'p':
			t.pasteRow()
			return true
		}
		return t.grid.HandleKey(event)
	}

	if event.Rune() == 'g' {
		t.gPressed = true
		return t.grid.HandleKey(event) // let DataGrid handle gg
	}

	// Ctrl key combinations
	switch event.Key() {
	case tcell.KeyCtrlY:
		t.copyRowAsInsert()
		return true
	case tcell.KeyCtrlE:
		t.showExportPicker()
		return true
	}

	switch event.Rune() {
	case 'n':
		t.nextPage()
		return true
	case 'N':
		t.prevPage()
		return true
	case 'd':
		t.dPressed = true
		return true
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
		return true
	case 'o', 'O':
		t.showInsertForm()
		return true
	case '.':
		t.repeatLastEdit()
		return true
	case 'p':
		t.toggleDetailPanel()
		return true
	case 'W':
		t.showCellDetail()
		return true
	case 'm':
		t.saveBookmark()
		return true
	case '\'':
		t.showBookmarkPicker()
		return true
	}
	return t.grid.HandleKey(event)
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
			filterInfo = fmt.Sprintf(" · [%s]filter: %s[-]", theme.TagWarning(), t.searchFilter)
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
	t.statusBar.SetText(fmt.Sprintf(" [%s]%s[-] · [%s]Ctrl+S to submit[-]",
		theme.TagWarning(), strings.Join(parts, " · "), theme.TagFgDim()))
}

func (t *TableData) loadData() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	type loadResult struct {
		columns []engine.ColumnInfo
		pkCols  []string
		fkInfo  []engine.ForeignKeyInfo
		result  *engine.QueryResult
	}

	async.NewLoader[loadResult]().
		WithTimeout(30 * time.Second).
		OnSuccess(func(lr loadResult) {
			t.columns = lr.columns
			t.pkCols = lr.pkCols
			t.fkInfo = lr.fkInfo
			t.setGridData(lr.result)
		}).
		OnError(func(err error) {
			t.app.ShowError(err.Error())
		}).
		Run(func(ctx context.Context) (loadResult, error) {
			// Get column info for PK detection
			columns, err := provider.DescribeTable(ctx, t.schema, t.table)
			if err != nil {
				return loadResult{}, fmt.Errorf("Describe table failed: %w", err)
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
				return loadResult{}, fmt.Errorf("Query failed: %w", err)
			}

			// Fetch FK info (non-critical, ignore errors)
			fkInfo, _ := provider.GetForeignKeys(ctx, t.schema, t.table)

			return loadResult{columns: columns, pkCols: pkCols, fkInfo: fkInfo, result: result}, nil
		})
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
	t.applyDeletionMarks()
	// Keyboard focus stays on the page stack; TableData.HandleKey is the page
	// router and forwards to the grid (incl. i/c/e to enter edit mode). Moving
	// app focus onto the grid here would bypass TableData.HandleKey entirely
	// (nav.Pages is not a key-routing Container), killing the table's own
	// keybinds and breaking inline editing's table-level integration.
	t.updateStatusBar()
	if t.detailVisible && !showEmpty {
		t.updateDetailPanel()
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
