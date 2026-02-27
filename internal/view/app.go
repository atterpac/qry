package view

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/jig/binding"
	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/layout"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/atterpac/jig/theme/themes"
	"github.com/atterpac/jig/validators"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
)

// CommandContextProvider is implemented by views that provide context for command expansion.
type CommandContextProvider interface {
	CommandContext() CommandViewContext
}

// CommandViewContext holds the view-specific variables for command template expansion.
type CommandViewContext struct {
	Table, Schema, Database, Engine, Query string
}

// JumpEntry records a navigation position for the jump list.
type JumpEntry struct {
	Schema string
	Table  string
	Filter string
}

// App is the main application controller.
type App struct {
	app       *layout.App
	statusBar *layout.StatusBar
	menu      *layout.Menu
	toasts    *components.ToastManager

	mu            sync.RWMutex
	provider      engine.Provider
	activeProfile *binding.Value[string]
	cfg           *config.Config
	history       *QueryHistoryStore
	gridEditing   bool // true when a DataGrid is in edit mode
	gridSearching bool // true when a table search filter is active

	// Jump list for Ctrl+O / Ctrl+I navigation
	jumpList   []JumpEntry
	jumpCursor int
}

// NewApp creates the application with a database provider.
func NewApp(provider engine.Provider, cfg *config.Config, activeProfileName string) *App {
	maxHistory := cfg.MaxHistory
	if maxHistory <= 0 {
		maxHistory = 100
	}
	a := &App{
		provider:      provider,
		cfg:           cfg,
		activeProfile: binding.NewValue(activeProfileName),
		history:       NewQueryHistoryStore(maxHistory),
	}
	a.buildApp()
	a.setup()
	return a
}

func (a *App) buildApp() {
	a.statusBar = layout.NewStatusBar()
	a.statusBar.SetTitle("qry")
	a.statusBar.SetTitleAlign(components.AlignLeft)
	a.menu = layout.NewMenu()

	a.app = layout.NewApp(layout.AppConfig{
		TopBar:          a.statusBar,
		BottomBar:       a.menu,
		ShowCrumbs:      true,
		TopBarHeight:    3,
		BottomBarHeight: 1,
		OnComponentChange: func(c nav.Component) {
			if c != nil {
				a.menu.SetHints(c.Hints())
			}
		},
	})

	// Initialize toast manager
	a.toasts = components.NewToastManager(a.app.GetApplication())
	a.toasts.SetPosition(components.ToastBottomRight)
	a.toasts.SetMaxVisible(3)
	a.toasts.SetDefaultDuration(3 * time.Second)

	a.app.GetApplication().SetAfterDrawFunc(func(screen tcell.Screen) {
		w, h := screen.Size()
		a.toasts.Draw(screen, w, h)
	})

	// Reactive binding for profile status — use Subscribe (not BindToWithDraw)
	// to avoid nested QueueUpdateDraw deadlocks when Set is called from within
	// a QueueUpdateDraw callback.
	a.updateProfileStatus(a.activeProfile.Get())
	a.activeProfile.Subscribe(func(_, newVal string) {
		a.updateProfileStatus(newVal)
	})
}

// updateProfileStatus refreshes the status bar sections for the given profile name.
// Safe to call from any goroutine context (does not trigger QueueUpdateDraw).
func (a *App) updateProfileStatus(profile string) {
	a.statusBar.ClearSections()
	colorFunc := theme.Get().Accent
	if strings.Contains(profile, "(connecting") {
		colorFunc = theme.Get().Warning
	} else if strings.Contains(profile, "(failed)") {
		colorFunc = theme.Get().Error
	}

	engineLabel := ""
	if a.provider != nil {
		engineLabel = string(a.provider.EngineType())
	}

	a.statusBar.AddSection(layout.StatusSection{
		Text:      profile,
		ColorFunc: colorFunc,
	})
	if engineLabel != "" {
		a.statusBar.AddSection(layout.StatusSection{
			Text:      engineLabel,
			ColorFunc: theme.Get().Fg,
		})
	}
}

func (a *App) setup() {
	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Skip global handling when command bar is active
		if a.statusBar.IsCommandMode() {
			return event
		}

		isModal := a.app.Pages().CurrentIsModal()

		switch {
		case event.Rune() == 'q' && !isModal:
			if !a.app.Pages().CanPop() {
				a.app.Stop()
				return nil
			}

		case event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyBackspace2:
			if a.gridEditing || a.gridSearching {
				return event // let the DataGrid handle it
			}
			if isModal {
				a.app.Pages().DismissModal()
				return nil
			}
			if a.app.Pages().CanPop() {
				a.app.Pages().Pop()
				return nil
			}

		case event.Rune() == '?' && !isModal:
			a.showHelp()
			return nil

		case event.Rune() == 'T' && !isModal:
			a.showThemeSelector()
			return nil

		case event.Rune() == 'P' && !isModal:
			a.showProfileSelector()
			return nil

		case event.Key() == tcell.KeyCtrlP && !isModal:
			a.showGlobalFinder()
			return nil

		case event.Rune() == ':' && !isModal:
			a.showCommandBar()
			return nil

		case event.Rune() == '\'' && !isModal:
			a.showBookmarkPicker()
			return nil

		case event.Key() == tcell.KeyCtrlO && !isModal:
			a.jumpBack()
			return nil

		case event.Key() == tcell.KeyCtrlI && !isModal:
			a.jumpForward()
			return nil
		}

		return event
	})

	// Push initial view
	if a.provider == nil {
		a.showFirstRunSetup()
	} else {
		a.NavigateToSchemaExplorer()
	}
}

// showFirstRunSetup displays a connection setup form for first-time users.
func (a *App) showFirstRunSetup() {
	form := components.NewFormBuilder().
		Text("name", "Profile Name").
			Value("default").
			Validate(validators.Required()).
			Done().
		Select("engine", "Engine", []string{"sqlite", "postgres", "mysql"}).
			Done().
		Text("dsn", "DSN").
			Placeholder("postgres://user:pass@host:5432/db").
			Done().
		Text("path", "Path").
			Placeholder("path/to/database.db").
			Done().
		OnSubmit(func(values map[string]any) {
			profileName := values["name"].(string)
			engineName := values["engine"].(string)
			dsn := values["dsn"].(string)
			path := values["path"].(string)

			connCfg := config.ConnectionConfig{
				Engine: config.EngineType(engineName),
				DSN:    dsn,
				Path:   path,
			}

			// Save profile to config
			a.cfg.SaveProfile(profileName, connCfg)
			if err := a.cfg.SetActiveProfile(profileName); err == nil {
				go a.cfg.Save()
			}

			// Connect in background (same pattern as switchProfile)
			a.activeProfile.Set(profileName + " (connecting...)")

			go func() {
				connCfg = connCfg.ExpandEnv()

				newProvider, err := engine.NewProvider(connCfg.Engine)
				if err != nil {
					a.QueueUpdateDraw(func() {
						a.activeProfile.Set(profileName + " (failed)")
						a.ShowError(fmt.Sprintf("Engine error: %v", err))
					})
					return
				}

				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()

				if err := newProvider.Connect(ctx, connCfg); err != nil {
					a.QueueUpdateDraw(func() {
						a.activeProfile.Set(profileName + " (failed)")
						a.ShowError(fmt.Sprintf("Connection failed: %v", err))
					})
					return
				}

				a.mu.Lock()
				a.provider = newProvider
				a.mu.Unlock()

				a.QueueUpdateDraw(func() {
					a.activeProfile.Set(profileName)
					a.app.Pages().Clear()
					a.NavigateToSchemaExplorer()
					a.ShowSuccess(fmt.Sprintf("Connected to %s", profileName))
				})
			}()
		}).
		OnCancel(func() {
			a.app.Stop()
		}).
		Build()

	modal := components.NewModal(components.ModalConfig{
		Title:    "Welcome to qry",
		Width:    60,
		Height:   16,
		Backdrop: true,
	})
	modal.SetContent(form)
	modal.SetHints([]components.KeyHint{
		{Key: "Tab", Description: "Next field"},
		{Key: "Ctrl+S", Description: "Connect"},
		{Key: "Esc", Description: "Quit"},
	})

	a.app.Pages().Push(modal)
	a.app.SetFocus(form)
}

// Run starts the TUI event loop.
func (a *App) Run() error {
	return a.app.Run()
}

// Provider returns the database provider (thread-safe).
func (a *App) Provider() engine.Provider {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.provider
}

// Config returns the config.
func (a *App) Config() *config.Config {
	return a.cfg
}

// ActiveProfileName returns the active profile name.
func (a *App) ActiveProfileName() string {
	return a.activeProfile.Get()
}

// QueueUpdateDraw queues a UI update and redraw (thread-safe).
func (a *App) QueueUpdateDraw(fn func()) {
	a.app.QueueUpdateDraw(fn)
}

// Suspend suspends the TUI for external command execution.
func (a *App) Suspend(fn func()) bool {
	return a.app.Suspend(fn)
}

// Navigation methods

func (a *App) NavigateToSchemaExplorer() {
	view := NewSchemaExplorer(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToTableData(schema, table string) {
	a.pushJumpEntry(schema, table, "")
	view := NewTableData(a, schema, table)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToTableDataWithFilter(schema, table, filter string) {
	a.pushJumpEntry(schema, table, filter)
	view := NewTableData(a, schema, table)
	view.SetFilter(filter)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToQueryEditor() {
	view := NewQueryEditor(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToQueryEditorWithSQL(sql string) {
	view := NewQueryEditorWithSQL(a, sql)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToConnectionInfo() {
	view := NewConnectionInfo(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToDatabaseList() {
	view := NewDatabaseList(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToQueryList() {
	view := NewQueryList(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToQueryHistory() {
	view := NewQueryHistory(a)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToERD(schema string) {
	view := NewErdView(a, schema)
	a.app.Pages().Push(view)
}

// History returns the query history store.
func (a *App) History() *QueryHistoryStore {
	return a.history
}

// Toast helpers

func (a *App) ShowSuccess(msg string) {
	a.toasts.Success(msg)
}

func (a *App) ShowError(msg string) {
	a.toasts.Error(msg)
}

func (a *App) ShowInfo(msg string) {
	a.toasts.Info(msg)
}

func (a *App) ShowWarning(msg string) {
	a.toasts.Warning(msg)
}

// UI helpers

// ShowSearchMode enters the status bar in search mode with `/` prompt.
// It wires live text changes, submit, and cancel to the provided callbacks.
func (a *App) ShowSearchMode(currentText string, callbacks components.SearchCallbacks) {
	a.statusBar.SetCommandPrompt("/ ")
	a.statusBar.SetCommandPlaceholder("search...")
	a.statusBar.SetOnComplete(nil)

	a.statusBar.EnterCommandMode()
	input := a.statusBar.GetCommandInput()
	if currentText != "" {
		input.SetText(currentText)
	}
	a.app.SetFocus(input)

	input.SetChangedFunc(func(text string) {
		if callbacks.OnChange != nil {
			callbacks.OnChange(text)
		}
	})

	a.statusBar.SetOnCommandSubmit(func(text string) {
		input.SetChangedFunc(nil)
		a.statusBar.ExitCommandMode()
		if callbacks.OnSubmit != nil {
			callbacks.OnSubmit(text)
		}
		a.refocusCurrent()
	})
	a.statusBar.SetOnCommandCancel(func() {
		input.SetChangedFunc(nil)
		a.statusBar.ExitCommandMode()
		if callbacks.OnCancel != nil {
			callbacks.OnCancel()
		}
		a.refocusCurrent()
	})
}

func (a *App) showCommandBar() {
	a.statusBar.SetCommandPrompt(": ")
	a.statusBar.SetCommandPlaceholder("command...")

	activeProfile, _ := a.cfg.GetActiveProfile()
	a.statusBar.SetOnComplete(func(input string) []string {
		builtins := []string{
			"tables", "schema", "editor", "e",
			"info", "databases", "db",
			"queries", "history",
			"run", "table",
			"sort", "count", "describe",
			"erd", "profile", "quit", "q",
		}
		userCmds := a.cfg.ListCommandNames(activeProfile)
		all := append(builtins, userCmds...)

		if input == "" {
			return all
		}
		var matches []string
		for _, name := range all {
			if strings.HasPrefix(name, input) {
				matches = append(matches, name)
			}
		}
		return matches
	})

	a.statusBar.EnterCommandMode()
	a.app.SetFocus(a.statusBar.GetCommandInput())

	a.statusBar.SetOnCommandSubmit(func(text string) {
		a.statusBar.ExitCommandMode()
		a.handleCommand(text)
		a.refocusCurrent()
	})
	a.statusBar.SetOnCommandCancel(func() {
		a.statusBar.ExitCommandMode()
		a.refocusCurrent()
	})
}

func (a *App) handleCommand(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	parts := strings.Fields(text)
	cmd := parts[0]
	args := parts[1:]

	switch cmd {
	case "tables", "schema":
		a.NavigateToSchemaExplorer()
	case "editor", "e":
		if len(args) > 0 {
			a.NavigateToQueryEditorWithSQL(strings.Join(args, " "))
		} else {
			a.NavigateToQueryEditor()
		}
	case "info":
		a.NavigateToConnectionInfo()
	case "databases", "db":
		a.NavigateToDatabaseList()
	case "queries":
		a.NavigateToQueryList()
	case "history":
		a.NavigateToQueryHistory()
	case "run":
		if len(args) > 0 {
			sql := strings.Join(args, " ")
			a.NavigateToQueryEditorWithSQL(sql)
		}
	case "table":
		if len(args) > 0 {
			a.NavigateToTableData("", args[0])
		}
	case "erd":
		schema := "public"
		if len(args) > 0 {
			schema = args[0]
		}
		a.NavigateToERD(schema)
	case "profile":
		a.showProfileSelector()
	case "quit", "q":
		a.app.Stop()
	case "sort":
		td, ok := a.currentTableData()
		if !ok {
			a.ShowWarning("Not viewing a table")
			return
		}
		if len(args) == 0 {
			a.ShowWarning("Usage: sort <column> [desc]")
			return
		}
		col := args[0]
		// Validate column exists
		found := false
		for _, rc := range td.resultCols {
			if strings.EqualFold(rc, col) {
				col = rc // use exact case
				found = true
				break
			}
		}
		if !found {
			a.ShowWarning(fmt.Sprintf("Column %q not found", col))
			return
		}
		dir := "ASC"
		if len(args) > 1 && strings.EqualFold(args[1], "desc") {
			dir = "DESC"
		}
		td.SetSort(col, dir)

	case "count":
		td, ok := a.currentTableData()
		if !ok {
			a.ShowWarning("Not viewing a table")
			return
		}
		td.runCount()

	case "describe", "desc":
		td, ok := a.currentTableData()
		if !ok {
			a.ShowWarning("Not viewing a table")
			return
		}
		td.showSchemaOverlay()

	default:
		a.ShowWarning(fmt.Sprintf("Unknown command: %s", cmd))
	}
}

// currentTableData returns the active TableData view if one is focused.
func (a *App) currentTableData() (*TableData, bool) {
	if c := a.app.Pages().Current(); c != nil {
		if td, ok := c.(*TableData); ok {
			return td, true
		}
	}
	return nil, false
}

// pushJumpEntry records a navigation for the jump list.
func (a *App) pushJumpEntry(schema, table, filter string) {
	// Truncate any forward entries
	if a.jumpCursor < len(a.jumpList) {
		a.jumpList = a.jumpList[:a.jumpCursor]
	}
	a.jumpList = append(a.jumpList, JumpEntry{
		Schema: schema,
		Table:  table,
		Filter: filter,
	})
	a.jumpCursor = len(a.jumpList)
}

func (a *App) jumpBack() {
	if a.jumpCursor <= 1 {
		a.ShowInfo("No previous location")
		return
	}
	a.jumpCursor--
	entry := a.jumpList[a.jumpCursor-1]
	view := NewTableData(a, entry.Schema, entry.Table)
	if entry.Filter != "" {
		view.SetFilter(entry.Filter)
	}
	a.app.Pages().Push(view)
}

func (a *App) jumpForward() {
	if a.jumpCursor >= len(a.jumpList) {
		a.ShowInfo("No next location")
		return
	}
	entry := a.jumpList[a.jumpCursor]
	a.jumpCursor++
	view := NewTableData(a, entry.Schema, entry.Table)
	if entry.Filter != "" {
		view.SetFilter(entry.Filter)
	}
	a.app.Pages().Push(view)
}

func (a *App) showBookmarkPicker() {
	bookmarks := a.cfg.Bookmarks
	tableBookmarks := make([]config.Bookmark, 0)
	for _, bm := range bookmarks {
		if bm.Type == "table" {
			tableBookmarks = append(tableBookmarks, bm)
		}
	}

	if len(tableBookmarks) == 0 {
		a.ShowInfo("No bookmarks saved (press m on a table to bookmark)")
		return
	}

	list := components.NewList().
		SetHighlightFullLine(true).
		SetWrapAround(true)

	for _, bm := range tableBookmarks {
		label := bm.Name
		if bm.Schema != "" {
			label = bm.Schema + "." + bm.Name
		}
		list.AddItem(label)
	}

	list.SetOnSelect(func(index int, _ components.ListItem) {
		a.app.Pages().Pop()
		bm := tableBookmarks[index]
		a.NavigateToTableData(bm.Schema, bm.Name)
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Bookmarks",
		Width:    50,
		Height:   min(len(tableBookmarks)+5, 15),
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Open"},
			{Key: "Esc", Description: "Close"},
		})

	a.app.Pages().Push(modal)
}

func (a *App) refocusCurrent() {
	if c := a.app.Pages().Current(); c != nil {
		a.app.SetFocus(c)
	}
}

func (a *App) showHelp() {
	helpText := `[::b]qry — Database Query Client[::-]

[::b]Global Hotkeys[::-]
  q          Quit / pop view
  Esc        Pop view / dismiss modal
  ?          This help
  T          Theme selector
  P          Profile selector
  :          Command bar
  '          Jump to bookmark
  Ctrl+O/I   Jump back/forward

[::b]Commands[::-]
  tables     Schema explorer
  editor     Query editor
  info       Connection info
  run <sql>  Execute SQL
  table <n>  Open table data
  sort <col> Sort by column
  count      Count rows
  describe   Show table schema
  profile    Switch profile
  quit       Exit`

	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetText(helpText)
	tv.SetTextAlign(tview.AlignLeft)

	modal := components.NewModal(components.ModalConfig{
		Title:  "Help",
		Width:  60,
		Height: 22,
	}).SetContent(tv)

	a.app.Pages().Push(modal)
}

func (a *App) showThemeSelector() {
	themeNames := themes.Names()
	currentTheme := a.cfg.Theme
	if currentTheme == "" {
		currentTheme = themes.DefaultName
	}

	selector := theme.NewThemeSelectorModal(themeNames, currentTheme)

	selector.SetOnPreview(func(name string) {
		if t := themes.Get(name); t != nil {
			theme.SetProvider(t)
		}
	})

	selector.SetOnSelect(func(name string) {
		if t := themes.Get(name); t != nil {
			theme.SetProvider(t)
			a.cfg.Theme = name
			go a.cfg.Save()
			a.app.Pages().Pop()
			a.ShowSuccess(fmt.Sprintf("Theme set to %s", name))
		}
	})

	selector.SetOnCancel(func() {
		if t := themes.Get(currentTheme); t != nil {
			theme.SetProvider(t)
		}
		a.app.Pages().Pop()
	})

	wrapper := &themeSelectorWrapper{selector: selector}
	a.app.Pages().Push(wrapper)
}

// themeSelectorWrapper wraps ThemeSelectorModal to implement nav.Component.
type themeSelectorWrapper struct {
	selector *theme.ThemeSelectorModal
}

func (w *themeSelectorWrapper) Name() string                    { return "Theme" }
func (w *themeSelectorWrapper) Start()                          {}
func (w *themeSelectorWrapper) Stop()                           {}
func (w *themeSelectorWrapper) Hints() []components.KeyHint     { return nil }
func (w *themeSelectorWrapper) Draw(screen tcell.Screen)        { w.selector.Draw(screen) }
func (w *themeSelectorWrapper) GetRect() (int, int, int, int)   { return w.selector.GetRect() }
func (w *themeSelectorWrapper) SetRect(x, y, width, height int) { w.selector.SetRect(x, y, width, height) }
func (w *themeSelectorWrapper) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return w.selector.InputHandler()
}
func (w *themeSelectorWrapper) Focus(delegate func(tview.Primitive)) { w.selector.Focus(delegate) }
func (w *themeSelectorWrapper) Blur()                                { w.selector.Blur() }
func (w *themeSelectorWrapper) HasFocus() bool                       { return w.selector.HasFocus() }
func (w *themeSelectorWrapper) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return w.selector.MouseHandler()
}
func (w *themeSelectorWrapper) PasteHandler() func(string, func(tview.Primitive)) { return nil }

func (a *App) showProfileSelector() {
	profiles := a.cfg.ListProfiles()
	if len(profiles) == 0 {
		a.ShowWarning("No profiles configured")
		return
	}

	list := components.NewList().
		SetHighlightFullLine(true).
		SetWrapAround(true)

	for _, name := range profiles {
		profile, _ := a.cfg.GetProfile(name)
		list.AddItem(fmt.Sprintf("%s  [%s]%s[-]", name, theme.TagFgDim(), string(profile.Engine)))
	}

	list.SetOnSelect(func(index int, _ components.ListItem) {
		a.app.Pages().Pop()
		a.switchProfile(profiles[index])
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Switch Profile",
		Width:    50,
		Height:   min(len(profiles)+5, 20),
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Connect"},
			{Key: "Esc", Description: "Close"},
		})

	a.app.Pages().Push(modal)
}

func (a *App) switchProfile(name string) {
	a.activeProfile.Set(name + " (connecting...)")

	go func() {
		profile, ok := a.cfg.GetProfile(name)
		if !ok {
			a.QueueUpdateDraw(func() {
				a.activeProfile.Set(name + " (failed)")
				a.ShowError(fmt.Sprintf("Profile %q not found", name))
			})
			return
		}

		profile = profile.ExpandEnv()

		newProvider, err := engine.NewProvider(profile.Engine)
		if err != nil {
			a.QueueUpdateDraw(func() {
				a.activeProfile.Set(name + " (failed)")
				a.ShowError(fmt.Sprintf("Engine error: %v", err))
			})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := newProvider.Connect(ctx, profile); err != nil {
			a.QueueUpdateDraw(func() {
				a.activeProfile.Set(name + " (failed)")
				a.ShowError(fmt.Sprintf("Connection failed: %v", err))
			})
			return
		}

		a.mu.Lock()
		oldProvider := a.provider
		a.provider = newProvider
		a.mu.Unlock()

		if oldProvider != nil {
			oldProvider.Close()
		}

		if err := a.cfg.SetActiveProfile(name); err == nil {
			go a.cfg.Save()
		}

		a.QueueUpdateDraw(func() {
			a.activeProfile.Set(name)
			a.ShowSuccess(fmt.Sprintf("Switched to %s", name))
			// Clear the stack and push fresh schema explorer
			a.app.Pages().Clear()
			a.NavigateToSchemaExplorer()
		})
	}()
}
