package view

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/binding"
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/help"
	"github.com/atterpac/dado/input"
	"github.com/atterpac/dado/layout"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/atterpac/dado/theme/themes"
	"github.com/atterpac/dado/validators"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/command"
	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
)

// PipeableView is implemented by views that can provide data for shell piping.
type PipeableView interface {
	BuildPipeData(format string) string
}

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
	textEditing   bool // true when a TextArea (e.g. query editor) has focus
	txMode        bool // true when an explicit transaction is active

	// Jump list for Ctrl+O / Ctrl+I navigation
	jumpList   []JumpEntry
	jumpCursor int

	// Global key actions (single source of truth for dispatch + help).
	actions *input.ActionRegistry

	// Command-bar registry (built lazily on first use).
	commands *command.Table
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
			// Reset keyboard focus to the page stack whenever the active page
			// changes. Views may move focus to an inner widget (e.g. a grid)
			// while active; without this, popping back would leave the
			// FocusManager pointing at a widget no longer in the tree, so key
			// events would never reach the new top view. Flows that focus a
			// specific widget (forms, finder) do so after Push, overriding this.
			a.app.SetFocus(a.app.Pages())
		},
	})

	// Initialize toast manager
	a.toasts = components.NewToastManager()
	a.toasts.SetPosition(components.ToastBottomRight)
	a.toasts.SetMaxVisible(3)
	a.toasts.SetDefaultDuration(3 * time.Second)

	a.app.GetApp().SetAfterDrawFunc(func(screen tcell.Screen) {
		w, h := screen.Size()
		a.toasts.Draw(screen, w, h)
	})

	// Wire the built-in live theme selector. Seed it with the saved theme and
	// persist selections back to config.
	current := a.cfg.Theme
	if current == "" {
		current = themes.DefaultName
	}
	a.app.EnableThemes(layout.ThemeOptions{
		Default: current,
		OnChange: func(name string) {
			a.cfg.Theme = name
			go a.cfg.Save()
			a.ShowSuccess(fmt.Sprintf("Theme set to %s", name))
		},
	})

	// Reactive binding for profile status — use Subscribe (not BindToWithDraw)
	// to avoid nested QueueUpdateDraw deadlocks when Set is called from within
	// a QueueUpdateDraw callback.
	a.updateProfileStatus(a.activeProfile.Get())
	a.activeProfile.Subscribe(func(_, newVal string) {
		a.updateProfileStatus(newVal)
	})
}

// showTxQuitConfirm shows a dialog when quitting with an active transaction.
func (a *App) showTxQuitConfirm() {
	list := components.NewList().
		SetHighlightFullLine(true).
		SetWrapAround(true)

	list.AddItem("Commit and quit")
	list.AddItem("Rollback and quit")
	list.AddItem("Cancel")

	list.SetOnSelect(func(index int, _ components.ListItem) {
		a.app.Pages().Pop()
		tp, ok := a.Provider().(engine.TransactionalProvider)
		if !ok {
			a.app.Stop()
			return
		}
		ctx := context.Background()
		switch index {
		case 0:
			tp.CommitTx(ctx)
			a.txMode = false
			a.app.Stop()
		case 1:
			tp.RollbackTx(ctx)
			a.txMode = false
			a.app.Stop()
		case 2:
			// cancel — do nothing
		}
	})

	modal := components.NewModal(components.ModalConfig{
		Title:    "Active Transaction",
		Width:    40,
		Height:   8,
		Backdrop: true,
	}).SetContent(list).
		SetHints([]components.KeyHint{
			{Key: "j/k", Description: "Navigate"},
			{Key: "Enter", Description: "Select"},
		})

	a.app.Pages().Push(modal)
}

// updateTxStatus adds or removes the TX indicator from the status bar.
func (a *App) updateTxStatus() {
	// Rebuild profile status which also handles TX indicator
	a.updateProfileStatus(a.activeProfile.Get())
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

	// TX indicator
	if a.txMode {
		a.statusBar.AddSection(layout.StatusSection{
			Text:      "TX",
			ColorFunc: theme.Get().Warning,
		})
	}
}

func (a *App) setup() {
	a.actions = input.NewActionRegistry().
		AddSimple("help", '?', "Help", a.showHelp).
		AddSimple("theme", 'T', "Theme selector", a.showThemeSelector).
		AddSimple("profile", 'P', "Profile selector", a.showProfileSelector).
		AddCtrl("finder", 'p', "Global finder", a.showGlobalFinder).
		AddSimple("command", ':', "Command bar", a.showCommandBar).
		AddSimple("bookmark", '\'', "Jump to bookmark", a.showBookmarkPicker).
		AddCtrl("jump-back", 'o', "Jump back", a.jumpBack).
		AddCtrl("jump-forward", 'i', "Jump forward", a.jumpForward)

	a.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Skip global handling when command bar is active
		if a.statusBar.IsCommandMode() {
			return event
		}

		// When editing a grid cell, pass all events through to the DataGrid
		// which handles text input, Escape (cancel), and Enter (confirm).
		if a.gridEditing {
			return event
		}

		// Skip rune-based shortcuts when editing text (query editor)
		if a.textEditing && event.Key() == tcell.KeyRune {
			return event
		}

		isModal := a.app.Pages().CurrentIsModal()

		switch {
		case event.Rune() == 'q' && !isModal:
			if !a.app.Pages().CanPop() {
				if a.txMode {
					a.showTxQuitConfirm()
					return nil
				}
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

		}

		// Simple global shortcuts live in the action registry, which is also
		// the source of truth for the help modal's "Global Hotkeys" section.
		if !isModal && a.actions.Handle(event) {
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

			async.NewLoader[engine.Provider]().
				WithTimeout(10 * time.Second).
				OnSuccess(func(newProvider engine.Provider) {
					a.mu.Lock()
					a.provider = newProvider
					a.mu.Unlock()

					a.activeProfile.Set(profileName)
					a.app.Pages().Clear()
					a.NavigateToSchemaExplorer()
					a.ShowSuccess(fmt.Sprintf("Connected to %s", profileName))
				}).
				OnError(func(err error) {
					a.activeProfile.Set(profileName + " (failed)")
					a.ShowError(err.Error())
				}).
				Run(func(ctx context.Context) (engine.Provider, error) {
					connCfg = connCfg.ExpandEnv()

					newProvider, err := engine.NewProvider(connCfg.Engine)
					if err != nil {
						return nil, fmt.Errorf("Engine error: %v", err)
					}

					if err := newProvider.Connect(ctx, connCfg); err != nil {
						return nil, fmt.Errorf("Connection failed: %v", err)
					}

					return newProvider, nil
				})
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

func (a *App) NavigateToSchemaDiff(targetProfile, schema string) {
	view := NewSchemaDiff(a, targetProfile, schema)
	a.app.Pages().Push(view)
}

func (a *App) NavigateToExplainView(sql string) {
	view := NewExplainView(a, sql)
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
	if currentText != "" {
		a.statusBar.SetCommandText(currentText)
	}
	a.app.SetFocus(a.statusBar)

	a.statusBar.SetOnCommandChange(func(text string) {
		if callbacks.OnChange != nil {
			callbacks.OnChange(text)
		}
	})

	a.statusBar.SetOnCommandSubmit(func(text string) {
		a.statusBar.SetOnCommandChange(nil)
		a.statusBar.ExitCommandMode()
		if callbacks.OnSubmit != nil {
			callbacks.OnSubmit(text)
		}
		a.refocusCurrent()
	})
	a.statusBar.SetOnCommandCancel(func() {
		a.statusBar.SetOnCommandChange(nil)
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
			"erd", "profile", "pipe",
			"begin", "commit", "rollback", "discard", "dry-run",
			"explain", "watch", "diff",
			"quit", "q",
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
	a.app.SetFocus(a.statusBar)

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
	if a.commands == nil {
		a.commands = a.buildCommandTable()
	}
	cmd, name, args, found := a.commands.Dispatch(text)
	if name == "" {
		return
	}
	if !found {
		a.ShowWarning(fmt.Sprintf("Unknown command: %s", name))
		return
	}
	cmd.Run(args)
}

// currentPipeableView returns the current view if it implements PipeableView.
func (a *App) currentPipeableView() PipeableView {
	if c := a.app.Pages().Current(); c != nil {
		if pv, ok := c.(PipeableView); ok {
			return pv
		}
	}
	return nil
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
	// Focus the page stack; key events route through Pages to the current
	// component's HandleKey.
	a.app.SetFocus(a.app.Pages())
}

// helpModel builds the help content: global hotkeys are derived from the
// action registry (single source of truth with key dispatch), with the few
// specially-handled keys and the command-bar commands added as static sections.
func (a *App) helpModel() *help.Help {
	return help.New().
		SetAppName("qry").
		AddSection("Navigation", []help.ActionInfo{
			{Key: "q", Description: "Quit / pop view"},
			{Key: "Esc", Description: "Pop view / dismiss modal"},
		}).
		AddRegistry("Global Hotkeys", a.actions).
		AddSection("Commands", []help.ActionInfo{
			{Key: "tables", Description: "Schema explorer"},
			{Key: "editor", Description: "Query editor"},
			{Key: "info", Description: "Connection info"},
			{Key: "run <sql>", Description: "Execute SQL"},
			{Key: "table <n>", Description: "Open table data"},
			{Key: "sort <col>", Description: "Sort by column"},
			{Key: "count", Description: "Count rows"},
			{Key: "describe", Description: "Show table schema"},
			{Key: "pipe", Description: "Pipe data to shell command"},
			{Key: "begin", Description: "Start transaction"},
			{Key: "commit", Description: "Commit transaction"},
			{Key: "rollback", Description: "Rollback transaction"},
			{Key: "discard", Description: "Discard pending edits/inserts/deletes"},
			{Key: "dry-run", Description: "Preview changes (rollback)"},
			{Key: "explain", Description: "Query plan visualization"},
			{Key: "watch <d>", Description: "Re-run query on interval"},
			{Key: "diff", Description: "Compare schemas between profiles"},
			{Key: "profile", Description: "Switch profile"},
			{Key: "quit", Description: "Exit"},
		})
}

func (a *App) showHelp() {
	var b strings.Builder
	b.WriteString("[::b]qry — Database Query Client[::-]\n")
	for _, section := range a.helpModel().GetSections() {
		b.WriteString("\n[::b]" + section.Name + "[::-]\n")
		for _, act := range section.Actions {
			fmt.Fprintf(&b, "  %-10s %s\n", act.Key, act.Description)
		}
	}

	tv := core.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetText(strings.TrimRight(b.String(), "\n"))
	tv.SetTextAlign(core.AlignLeft)

	modal := components.NewModal(components.ModalConfig{
		Title:  "Help",
		Width:  60,
		Height: 26,
	}).SetContent(tv)

	a.app.Pages().Push(modal)
}

func (a *App) showThemeSelector() {
	// The live-preview selector is wired via layout.App.EnableThemes in buildApp.
	a.app.OpenThemeSelector()
}

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

	async.NewLoader[engine.Provider]().
		WithTimeout(10 * time.Second).
		OnSuccess(func(newProvider engine.Provider) {
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

			a.activeProfile.Set(name)
			a.ShowSuccess(fmt.Sprintf("Switched to %s", name))
			// Clear the stack and push fresh schema explorer
			a.app.Pages().Clear()
			a.NavigateToSchemaExplorer()
		}).
		OnError(func(err error) {
			a.activeProfile.Set(name + " (failed)")
			a.ShowError(err.Error())
		}).
		Run(func(ctx context.Context) (engine.Provider, error) {
			profile, ok := a.cfg.GetProfile(name)
			if !ok {
				return nil, fmt.Errorf("Profile %q not found", name)
			}

			profile = profile.ExpandEnv()

			newProvider, err := engine.NewProvider(profile.Engine)
			if err != nil {
				return nil, fmt.Errorf("Engine error: %v", err)
			}

			if err := newProvider.Connect(ctx, profile); err != nil {
				return nil, fmt.Errorf("Connection failed: %v", err)
			}

			return newProvider, nil
		})
}
