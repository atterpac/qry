package view

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/atterpac/qry/internal/autocomplete"
	"github.com/atterpac/qry/internal/engine"
	"github.com/atterpac/qry/internal/lsp"
)

// QueryEditor is a split view with a TextArea for SQL and a DataGrid for results.
type QueryEditor struct {
	*components.Split
	app        *App
	editor     *tview.TextArea
	grid       *components.DataGrid
	source     *components.SliceSource
	statusBar  *tview.TextView
	resultPane *tview.Flex
	emptyState *components.EmptyState
	lastSQL    string
	wrapper    *tview.Flex
	hasResults bool

	// Autocomplete
	overlay    *autocomplete.Overlay
	acEngine   *autocomplete.SuggestionEngine
	acCache    *autocomplete.SchemaCache
	suppressAC bool
	schemaOverlay *schemaInfoOverlay
}

func NewQueryEditor(app *App) *QueryEditor {
	return newQueryEditor(app, "")
}

func NewQueryEditorWithSQL(app *App, sql string) *QueryEditor {
	return newQueryEditor(app, sql)
}

func newQueryEditor(app *App, initialSQL string) *QueryEditor {
	cache := autocomplete.NewSchemaCache(5 * time.Minute)
	q := &QueryEditor{
		app:       app,
		editor:    tview.NewTextArea(),
		source:    components.NewSliceSource(nil, nil).SetReadOnly(true),
		statusBar: tview.NewTextView(),
		acCache:   cache,
		acEngine:  autocomplete.NewSuggestionEngine(cache, "public"),
	}

	q.overlay = autocomplete.NewOverlay()
	q.overlay.OnAccept = func(s autocomplete.Suggestion) {
		q.acceptSuggestion(s)
	}
	q.overlay.OnDismiss = func() {}

	q.schemaOverlay = newSchemaInfoOverlay()

	q.editor.SetPlaceholder("Enter SQL query... (Ctrl+J to execute)")
	q.editor.SetBorder(true)
	q.editor.SetTitle(" SQL ")
	q.editor.SetTitleAlign(tview.AlignLeft)
	theme.Register(q.editor)
	if initialSQL != "" {
		q.editor.SetText(initialSQL, true)
	}

	q.grid = components.NewDataGrid()
	q.grid.SetShowRowNumbers(true)
	q.grid.SetShowHeader(true)
	q.grid.SetSource(q.source)
	q.grid.SetOnModeChange(func(mode components.GridMode) {
		app.gridEditing = mode == components.GridModeEdit
	})

	q.statusBar.SetDynamicColors(true)
	q.statusBar.SetText(fmt.Sprintf(" [%s]Ready[-]", theme.TagFgDim()))
	theme.Register(q.statusBar)

	// Empty state shown before any query is executed
	q.emptyState = components.NewEmptyState().
		Configure("󰍉", "No Results", "Execute a query with Ctrl+R")

	// Wrap grid + status in a flex
	q.resultPane = tview.NewFlex().SetDirection(tview.FlexRow)
	theme.Register(q.resultPane)
	q.resultPane.AddItem(q.emptyState, 0, 1, false)
	q.resultPane.AddItem(q.statusBar, 1, 0, false)

	q.Split = components.NewSplit().
		SetDirection(components.SplitVertical).
		SetTop(q.editor).
		SetBottom(q.resultPane).
		SetRatio(0.35).
		SetResizable(true)

	q.editor.SetChangedFunc(func() {
		q.updateSuggestions()
	})

	q.editor.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Schema info overlay: Shift+K
		if q.schemaOverlay.visible {
			if event.Key() == tcell.KeyEscape || (event.Rune() == 'K' && event.Modifiers() == tcell.ModNone) {
				q.schemaOverlay.Hide()
				return nil
			}
			if event.Key() == tcell.KeyUp || event.Key() == tcell.KeyDown {
				q.schemaOverlay.HandleKey(event)
				return nil
			}
			// Any other key dismisses and passes through
			q.schemaOverlay.Hide()
		}
		if event.Rune() == 'K' && event.Modifiers() == tcell.ModShift {
			q.showSchemaInfo()
			return nil
		}

		// Autocomplete overlay input handling
		if q.overlay.Visible() {
			if q.overlay.HandleKey(event) {
				return nil
			}
			// If the key wasn't consumed by overlay, let it pass through
			// but dismiss overlay on certain keys
			if event.Key() == tcell.KeyLeft || event.Key() == tcell.KeyRight {
				q.overlay.Hide()
			}
		}

		// Ctrl+R to execute
		if event.Key() == tcell.KeyCtrlR {
			q.overlay.Hide()
			q.suppressAC = true
			q.executeQuery()
			return nil
		}
		// Ctrl+E to open $EDITOR
		if event.Key() == tcell.KeyCtrlE {
			q.openExternalEditor()
			return nil
		}
		// Ctrl+S to save query
		if event.Key() == tcell.KeyCtrlS {
			q.saveQuery()
			return nil
		}
		// Tab to switch to results (only when overlay is NOT visible)
		if event.Key() == tcell.KeyTab {
			q.overlay.Hide()
			q.suppressAC = true
			q.Split.FocusSecond()
			return nil
		}
		return event
	})

	q.grid.SetOnBack(func() {
		q.Split.FocusFirst()
	})

	return q
}

func (q *QueryEditor) Name() string { return "Query Editor" }

func (q *QueryEditor) Start() {
	q.Split.FocusFirst()
	if q.editor.GetText() != "" {
		q.executeQuery()
	}
	// Warm autocomplete cache in background
	if provider := q.app.Provider(); provider != nil {
		go q.acCache.Warm(context.Background(), provider, q.acEngine.Schema)
	}
}

func (q *QueryEditor) Stop() {}

// Draw renders the split view and then draws the autocomplete overlay on top.
func (q *QueryEditor) Draw(screen tcell.Screen) {
	q.Split.Draw(screen)
	q.overlay.Draw(screen)
	q.schemaOverlay.Draw(screen)
}

func (q *QueryEditor) Hints() []components.KeyHint {
	tabDesc := "Switch pane"
	if q.overlay.Visible() {
		tabDesc = "Complete"
	}
	return []components.KeyHint{
		{Key: "Ctrl+R", Description: "Execute query"},
		{Key: "Ctrl+E", Description: "$EDITOR"},
		{Key: "Ctrl+S", Description: "Save query"},
		{Key: "Shift+K", Description: "Schema info"},
		{Key: "Tab", Description: tabDesc},
	}
}

func (q *QueryEditor) executeQuery() {
	provider := q.app.Provider()
	if provider == nil {
		q.app.ShowError("Not connected")
		return
	}

	sql := q.editor.GetText()
	if sql == "" {
		return
	}

	q.lastSQL = sql
	q.statusBar.SetText(fmt.Sprintf(" [yellow]Executing...[-]"))

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		start := time.Now()
		result, err := provider.ExecuteQuery(ctx, sql)
		elapsed := time.Since(start)

		// Record to history
		if q.app.History() != nil {
			entry := HistoryEntry{
				Query:    sql,
				Duration: elapsed,
				Time:     start,
			}
			if err != nil {
				entry.Error = err.Error()
			} else {
				entry.RowCount = result.RowCount
			}
			q.app.History().Add(entry)
		}

		if err != nil {
			q.app.QueueUpdateDraw(func() {
				q.statusBar.SetText(fmt.Sprintf(" [red]Error: %v[-]", err))
				q.app.ShowError(fmt.Sprintf("Query error: %v", err))
			})
			return
		}

		q.app.QueueUpdateDraw(func() {
			if result.Message != "" {
				q.showEmptyState("󰄬", "Query Executed", result.Message)
				q.statusBar.SetText(fmt.Sprintf(" [green]%s[-] [%s](%s)[-]",
					result.Message, theme.TagFgDim(), result.Duration))
			} else if len(result.Columns) > 0 {
				q.showGrid()
				q.source.SetSliceData(result.Columns, result.Rows)
				if result.RowCount == 0 {
					q.showEmptyState("󰍉", "No Rows", "Query returned no results")
				}
				q.statusBar.SetText(fmt.Sprintf(" [green]%d rows[-] [%s](%s)[-]",
					result.RowCount, theme.TagFgDim(), result.Duration))
			} else {
				q.showEmptyState("󰄬", "OK", "Query completed successfully")
				q.statusBar.SetText(fmt.Sprintf(" [green]OK[-] [%s](%s)[-]",
					theme.TagFgDim(), result.Duration))
			}
			q.Split.FocusSecond()
		})
	}()
}

func (q *QueryEditor) openExternalEditor() {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Write current SQL to temp file
	tmpFile, err := os.CreateTemp("", "qry-*.sql")
	if err != nil {
		q.app.ShowError(fmt.Sprintf("Create temp file: %v", err))
		return
	}
	tmpPath := tmpFile.Name()

	currentSQL := q.editor.GetText()
	if _, err := tmpFile.WriteString(currentSQL); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		q.app.ShowError(fmt.Sprintf("Write temp file: %v", err))
		return
	}
	tmpFile.Close()

	// Start LSP server and write init.lua for neovim
	var lspServer *lsp.LSPServer
	var initLuaPath string
	useNvimLSP := isNeovim(editor)

	if useNvimLSP {
		if provider := q.app.Provider(); provider != nil {
			lspServer = lsp.NewLSPServer(q.acCache, q.acEngine, provider)
			sockPath, err := lspServer.Start()
			if err == nil {
				initLuaPath, err = lsp.WriteInitLua(sockPath)
				if err != nil {
					lspServer.Shutdown()
					lspServer = nil
				}
			} else {
				lspServer = nil
			}
		}
	}

	// Suspend TUI and open editor
	q.app.Suspend(func() {
		var cmd *exec.Cmd
		if useNvimLSP && initLuaPath != "" {
			cmd = exec.Command(editor, "--cmd", "luafile "+initLuaPath, tmpPath)
		} else {
			cmd = exec.Command(editor, tmpPath)
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	})

	// Shutdown LSP and clean up
	if lspServer != nil {
		lspServer.Shutdown()
	}
	if initLuaPath != "" {
		os.Remove(initLuaPath)
	}

	// Read back the edited file
	data, err := os.ReadFile(tmpPath)
	os.Remove(tmpPath)
	if err != nil {
		q.app.ShowError(fmt.Sprintf("Read temp file: %v", err))
		return
	}

	q.suppressAC = true
	q.editor.SetText(string(data), true)
}

// isNeovim returns true if the editor command refers to neovim.
func isNeovim(editor string) bool {
	return strings.Contains(editor, "nvim")
}

func (q *QueryEditor) saveQuery() {
	sql := q.editor.GetText()
	if sql == "" {
		q.app.ShowInfo("Nothing to save")
		return
	}

	// Simple inline save — prompt for name via command bar approach
	input := tview.NewInputField()
	input.SetLabel("Query name: ")
	input.SetFieldWidth(40)
	input.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEnter {
			name := input.GetText()
			if name != "" {
				profileName := q.app.ActiveProfileName()
				q.app.Config().SavedQueryForProfile(profileName, name, sql)
				go q.app.Config().Save()
				q.app.ShowSuccess(fmt.Sprintf("Saved query: %s", name))
			}
		}
		q.app.app.Pages().Pop()
	})

	modal := components.NewModal(components.ModalConfig{
		Title:  "Save Query",
		Width:  50,
		Height: 5,
	}).SetContent(input)

	q.app.app.Pages().Push(modal)
}

// showGrid swaps the empty state for the data grid in the result pane.
func (q *QueryEditor) showGrid() {
	if q.hasResults {
		return
	}
	q.hasResults = true
	q.resultPane.Clear()
	q.resultPane.AddItem(q.grid, 0, 1, true)
	q.resultPane.AddItem(q.statusBar, 1, 0, false)
}

// showEmptyState swaps the data grid for an empty state message.
func (q *QueryEditor) showEmptyState(icon, title, message string) {
	q.hasResults = false
	q.emptyState.Configure(icon, title, message)
	q.resultPane.Clear()
	q.resultPane.AddItem(q.emptyState, 0, 1, false)
	q.resultPane.AddItem(q.statusBar, 1, 0, false)
}

// updateSuggestions runs the autocomplete engine and updates the overlay.
func (q *QueryEditor) updateSuggestions() {
	if q.suppressAC {
		q.suppressAC = false
		q.overlay.Hide()
		return
	}

	provider := q.app.Provider()
	if provider == nil {
		q.overlay.Hide()
		return
	}

	sql := q.editor.GetText()
	if sql == "" {
		q.overlay.Hide()
		return
	}

	cursorBytePos := q.cursorByteOffset(sql)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	suggestions := q.acEngine.Suggest(ctx, provider, sql, cursorBytePos)
	if len(suggestions) == 0 {
		q.overlay.Hide()
		return
	}

	x, y := q.cursorScreenPos()
	q.overlay.Show(suggestions, x, y)
}

// acceptSuggestion inserts the accepted suggestion into the editor.
func (q *QueryEditor) acceptSuggestion(s autocomplete.Suggestion) {
	sql := q.editor.GetText()
	cursorBytePos := q.cursorByteOffset(sql)
	pr := autocomplete.ParseContext(sql, cursorBytePos)

	insertText := s.InsertText
	if insertText == "" {
		insertText = s.Text
	}

	// If no partial token, check if we need a leading space
	if pr.PartialToken == "" && cursorBytePos > 0 && sql[cursorBytePos-1] != ' ' && sql[cursorBytePos-1] != '\n' && sql[cursorBytePos-1] != '\t' {
		insertText = " " + insertText
	}

	// Convert byte offsets to character positions for Replace()
	partialStartChar := byteOffsetToCharPos(sql, pr.PartialStart)
	cursorChar := byteOffsetToCharPos(sql, cursorBytePos)

	q.editor.Replace(partialStartChar, cursorChar, insertText)
}

// cursorByteOffset converts the TextArea cursor (row, col) to a byte offset in the SQL string.
func (q *QueryEditor) cursorByteOffset(sql string) int {
	_, _, toRow, toCol := q.editor.GetCursor()

	lines := strings.Split(sql, "\n")
	offset := 0
	for i := 0; i < toRow && i < len(lines); i++ {
		offset += len(lines[i]) + 1 // +1 for newline
	}
	if toRow < len(lines) {
		line := lines[toRow]
		// toCol is in terms of characters (runes), convert to bytes
		col := 0
		for bi := 0; bi < len(line) && col < toCol; {
			_, size := utf8.DecodeRuneInString(line[bi:])
			bi += size
			col++
			if col == toCol {
				offset += bi
				return offset
			}
		}
		// If we didn't reach toCol, add full line length
		if col < toCol {
			offset += len(line)
		}
	}
	return offset
}

// cursorScreenPos returns the screen coordinates of the cursor in the editor.
func (q *QueryEditor) cursorScreenPos() (int, int) {
	_, _, toRow, toCol := q.editor.GetCursor()
	rowOffset, colOffset := q.editor.GetOffset()

	// The editor's inner rect gives us the text area position
	x, y, _, _ := q.editor.GetInnerRect()

	screenX := x + toCol - colOffset
	screenY := y + toRow - rowOffset

	return screenX, screenY
}

// byteOffsetToCharPos converts a byte offset in a string to a character (rune) position.
func byteOffsetToCharPos(s string, byteOffset int) int {
	if byteOffset >= len(s) {
		return utf8.RuneCountInString(s)
	}
	return utf8.RuneCountInString(s[:byteOffset])
}

// showSchemaInfo shows column info for the table/identifier under/near the cursor.
func (q *QueryEditor) showSchemaInfo() {
	provider := q.app.Provider()
	if provider == nil {
		return
	}

	sql := q.editor.GetText()
	cursorBytePos := q.cursorByteOffset(sql)

	// Find the identifier at or near the cursor
	tableName, schema := q.findTableAtCursor(sql, cursorBytePos)
	if tableName == "" {
		q.app.ShowInfo("No table found at cursor")
		return
	}

	if schema == "" {
		schema = q.acEngine.Schema
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cols := q.acCache.Columns(ctx, provider, schema, tableName)
	if len(cols) == 0 {
		q.app.ShowInfo(fmt.Sprintf("No columns found for %s.%s", schema, tableName))
		return
	}

	x, y := q.cursorScreenPos()
	q.schemaOverlay.Show(tableName, schema, cols, x, y)
}

// findTableAtCursor finds the table name at the cursor position.
func (q *QueryEditor) findTableAtCursor(sql string, cursorBytePos int) (string, string) {
	tokens := autocomplete.Tokenize(sql)

	// Find the token at or immediately before the cursor
	for _, t := range tokens {
		if t.Start <= cursorBytePos && t.End >= cursorBytePos {
			if t.Type == autocomplete.TokenIdentifier || t.Type == autocomplete.TokenKeyword {
				// Check if this is a known table in current context
				pr := autocomplete.ParseContext(sql, cursorBytePos)
				// Check table refs first
				for _, ref := range pr.TableRefs {
					if strings.EqualFold(ref.Name, t.Value) || strings.EqualFold(ref.Alias, t.Value) {
						return ref.Name, ref.Schema
					}
				}
				// Fall back to using the token value as a table name
				return t.Value, ""
			}
		}
	}
	return "", ""
}

// schemaInfoOverlay displays table column information in a floating box.
type schemaInfoOverlay struct {
	visible      bool
	tableName    string
	schema       string
	columns      []engine.ColumnInfo
	anchorX      int
	anchorY      int
	scrollOffset int
	maxVisible   int
}

func newSchemaInfoOverlay() *schemaInfoOverlay {
	return &schemaInfoOverlay{maxVisible: 15}
}

func (o *schemaInfoOverlay) Show(table, schema string, cols []engine.ColumnInfo, x, y int) {
	o.tableName = table
	o.schema = schema
	o.columns = cols
	o.anchorX = x
	o.anchorY = y
	o.scrollOffset = 0
	o.visible = true
}

func (o *schemaInfoOverlay) Hide() {
	o.visible = false
	o.columns = nil
}

func (o *schemaInfoOverlay) HandleKey(event *tcell.EventKey) {
	switch event.Key() {
	case tcell.KeyUp:
		if o.scrollOffset > 0 {
			o.scrollOffset--
		}
	case tcell.KeyDown:
		max := len(o.columns) - o.maxVisible
		if max < 0 {
			max = 0
		}
		if o.scrollOffset < max {
			o.scrollOffset++
		}
	}
}

func (o *schemaInfoOverlay) Draw(screen tcell.Screen) {
	if !o.visible || len(o.columns) == 0 {
		return
	}

	screenW, screenH := screen.Size()

	// Calculate widths
	maxNameW := len("Column")
	maxTypeW := len("Type")
	for _, c := range o.columns {
		if len(c.Name) > maxNameW {
			maxNameW = len(c.Name)
		}
		dt := c.DataType
		if c.IsPrimaryKey {
			dt += " PK"
		}
		if !c.Nullable {
			dt += " NOT NULL"
		}
		if len(dt) > maxTypeW {
			maxTypeW = len(dt)
		}
	}

	title := fmt.Sprintf(" %s.%s ", o.schema, o.tableName)
	contentW := maxNameW + maxTypeW + 5 // " name │ type "
	if len(title) > contentW {
		contentW = len(title)
	}

	visibleCount := len(o.columns)
	if visibleCount > o.maxVisible {
		visibleCount = o.maxVisible
	}

	boxW := contentW + 2
	boxH := visibleCount + 4 // border + title + header + border

	if boxW > screenW-2 {
		boxW = screenW - 2
	}

	x := o.anchorX
	y := o.anchorY - boxH // show above cursor
	if y < 0 {
		y = o.anchorY + 1 // flip below
	}
	if x+boxW > screenW {
		x = screenW - boxW
	}
	if x < 0 {
		x = 0
	}
	if y+boxH > screenH {
		y = screenH - boxH
	}

	bgColor := theme.BgLight()
	fgColor := theme.Fg()
	borderColor := theme.Border()
	accentColor := theme.Accent()
	dimColor := theme.FgDim()

	borderStyle := tcell.StyleDefault.Background(bgColor).Foreground(borderColor)
	titleStyle := tcell.StyleDefault.Background(bgColor).Foreground(accentColor).Bold(true)
	headerStyle := tcell.StyleDefault.Background(bgColor).Foreground(accentColor)
	normalStyle := tcell.StyleDefault.Background(bgColor).Foreground(fgColor)
	pkStyle := tcell.StyleDefault.Background(bgColor).Foreground(theme.Warning())
	dimStyle := tcell.StyleDefault.Background(bgColor).Foreground(dimColor)

	// Draw box
	autocomplete.DrawBox(screen, x, y, boxW, boxH, borderStyle)

	// Draw title
	col := x + 1
	col = autocomplete.DrawString(screen, col, y, title, titleStyle)

	// Header row
	headerY := y + 1
	col = x + 1
	col = autocomplete.DrawString(screen, col, headerY, " ", headerStyle)
	col = autocomplete.DrawString(screen, col, headerY, padRight("Column", maxNameW), headerStyle)
	col = autocomplete.DrawString(screen, col, headerY, " │ ", borderStyle)
	col = autocomplete.DrawString(screen, col, headerY, padRight("Type", maxTypeW), headerStyle)
	// Fill rest
	for col < x+boxW-1 {
		screen.SetContent(col, headerY, ' ', nil, headerStyle)
		col++
	}

	// Separator
	sepY := y + 2
	for col := x + 1; col < x+boxW-1; col++ {
		screen.SetContent(col, sepY, '─', nil, borderStyle)
	}

	// Column rows
	for vi := 0; vi < visibleCount; vi++ {
		idx := o.scrollOffset + vi
		if idx >= len(o.columns) {
			break
		}

		c := o.columns[idx]
		row := y + 3 + vi

		// Fill background
		for col := x + 1; col < x+boxW-1; col++ {
			screen.SetContent(col, row, ' ', nil, normalStyle)
		}

		style := normalStyle
		if c.IsPrimaryKey {
			style = pkStyle
		}

		col := x + 1
		col = autocomplete.DrawString(screen, col, row, " ", style)
		col = autocomplete.DrawString(screen, col, row, padRight(c.Name, maxNameW), style)
		col = autocomplete.DrawString(screen, col, row, " │ ", borderStyle)

		dt := c.DataType
		if c.IsPrimaryKey {
			dt += " PK"
		}
		if !c.Nullable {
			dt += " NOT NULL"
		}
		autocomplete.DrawString(screen, col, row, padRight(dt, maxTypeW), dimStyle)
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// CommandContext implements CommandContextProvider.
func (q *QueryEditor) CommandContext() CommandViewContext {
	ctx := CommandViewContext{
		Query: q.lastSQL,
	}
	if q.app.Provider() != nil {
		ctx.Engine = string(q.app.Provider().EngineType())
	}
	return ctx
}

var _ nav.Component = (*QueryEditor)(nil)
