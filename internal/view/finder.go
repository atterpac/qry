package view

import (
	"context"
	"time"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/components"
)

// finderWrapper wraps components.Finder to implement nav.Component for page stack.
type finderWrapper struct {
	*components.Finder
}

func (w *finderWrapper) Name() string                { return "Find" }
func (w *finderWrapper) Start()                      {}
func (w *finderWrapper) Stop()                       {}
func (w *finderWrapper) Hints() []components.KeyHint { return nil }

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

	wrapper := &finderWrapper{Finder: finder}
	a.app.Pages().Push(wrapper)
	a.app.SetFocus(finder)

	// Fetch items in background
	async.NewLoader[[]components.FinderItem]().
		WithTimeout(10 * time.Second).
		OnSuccess(func(items []components.FinderItem) {
			finder.SetItems(items)
		}).
		OnError(func(err error) {}).
		Run(func(ctx context.Context) ([]components.FinderItem, error) {
			var items []components.FinderItem

			provider := a.Provider()
			if provider == nil {
				return items, nil
			}

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

			return items, nil
		})
}
