package view

import (
	"context"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// finderWrapper wraps components.Finder to implement nav.Component for page stack.
type finderWrapper struct {
	finder *components.Finder
}

func (w *finderWrapper) Name() string                    { return "Find" }
func (w *finderWrapper) Start()                          {}
func (w *finderWrapper) Stop()                           {}
func (w *finderWrapper) Hints() []components.KeyHint     { return nil }
func (w *finderWrapper) Draw(screen tcell.Screen)        { w.finder.Draw(screen) }
func (w *finderWrapper) GetRect() (int, int, int, int)   { return w.finder.GetRect() }
func (w *finderWrapper) SetRect(x, y, width, height int) { w.finder.SetRect(x, y, width, height) }
func (w *finderWrapper) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return w.finder.InputHandler()
}
func (w *finderWrapper) Focus(delegate func(tview.Primitive))        { w.finder.Focus(delegate) }
func (w *finderWrapper) Blur()                                       { w.finder.Blur() }
func (w *finderWrapper) HasFocus() bool                              { return w.finder.HasFocus() }
func (w *finderWrapper) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return w.finder.MouseHandler()
}
func (w *finderWrapper) PasteHandler() func(string, func(tview.Primitive)) { return nil }

// showGlobalFinder opens the global fuzzy finder (Ctrl+P).
func (a *App) showGlobalFinder() {
	finder := components.NewFinder().
		SetPlaceholder("Search tables, saved queries...").
		SetPrompt("> ").
		SetShowCategories(true).
		SetShowDescription(true).
		SetMaxVisible(15).
		SetVimMode(true)

	finder.SetCategories([]components.FinderCategory{
		{Name: "Tables", Icon: "󰓫", Priority: 1},
		{Name: "Saved Queries", Icon: "󰬔", Priority: 2},
	})

	finder.SetOnSelect(func(item components.FinderItem) {
		a.app.Pages().Pop()
		switch item.Category {
		case "Tables":
			a.NavigateToTableData("", item.ID)
		case "Saved Queries":
			a.NavigateToQueryEditorWithSQL(item.Description)
		}
	})

	finder.SetOnCancel(func() {
		a.app.Pages().Pop()
	})

	wrapper := &finderWrapper{finder: finder}
	a.app.Pages().Push(wrapper)
	a.app.SetFocus(finder)

	// Fetch items in background
	go func() {
		var items []components.FinderItem

		provider := a.Provider()
		if provider == nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Tables
		tables, err := provider.ListTables(ctx, "")
		if err == nil {
			for _, t := range tables {
				items = append(items, components.FinderItem{
					ID:          t.Name,
					Label:       t.Name,
					Description: t.Type,
					Category:    "Tables",
				})
			}
		}

		// Saved queries
		profileName := a.ActiveProfileName()
		if profile, ok := a.Config().GetProfile(profileName); ok {
			for _, sq := range profile.SavedQueries {
				items = append(items, components.FinderItem{
					ID:          sq.Name,
					Label:       sq.Name,
					Description: sq.Query,
					Category:    "Saved Queries",
				})
			}
		}

		a.QueueUpdateDraw(func() {
			finder.SetItems(items)
		})
	}()
}
