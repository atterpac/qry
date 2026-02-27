package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/atterpac/qry/internal/config"
)

// QueryList is a MasterDetailView showing saved queries and SQL preview.
type QueryList struct {
	*components.MasterDetailView
	app         *App
	queryTable  *tview.Table
	preview     *tview.TextView
	queries     []config.SavedQuery
	filtered    []config.SavedQuery
	searchQuery string
}

func NewQueryList(app *App) *QueryList {
	q := &QueryList{
		app:        app,
		queryTable: tview.NewTable(),
		preview:    tview.NewTextView(),
	}

	q.queryTable.SetSelectable(true, false)
	q.queryTable.SetFixed(1, 0)
	q.queryTable.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))
	theme.Register(q.queryTable)

	q.preview.SetDynamicColors(true)
	q.preview.SetWordWrap(true)
	theme.Register(q.preview)

	q.MasterDetailView = components.NewMasterDetailView().
		SetMasterTitle("Saved Queries").
		SetDetailTitle("Preview").
		SetMasterContent(q.queryTable).
		SetDetailContent(q.preview).
		SetRatio(0.4).
		SetResizable(true)

	q.MasterDetailView.ConfigureEmpty("󰬔", "No Saved Queries", "Save queries from the editor with Ctrl+S")

	q.MasterDetailView.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Open in editor"},
		{Key: "r", Description: "Run directly"},
		{Key: "d", Description: "Delete"},
		{Key: "/", Description: "Search"},
	})

	q.MasterDetailView.EnableSearch(func(currentText string, callbacks components.SearchCallbacks) {
		app.ShowSearchMode(currentText, callbacks)
	})
	q.MasterDetailView.SetOnSearch(func(query string) {
		q.searchQuery = query
		q.applyFilter()
	})
	q.MasterDetailView.SetOnSearchCancel(func() {
		q.searchQuery = ""
		q.applyFilter()
	})

	q.queryTable.SetSelectionChangedFunc(func(row, col int) {
		q.onSelectionChanged(row)
	})

	q.queryTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if q.MasterDetailView.HandleSearchKey(event) {
			return nil
		}

		switch event.Rune() {
		case 'j':
			row, _ := q.queryTable.GetSelection()
			if row < q.queryTable.GetRowCount()-1 {
				q.queryTable.Select(row+1, 0)
			}
			return nil
		case 'k':
			row, _ := q.queryTable.GetSelection()
			if row > 1 {
				q.queryTable.Select(row-1, 0)
			}
			return nil
		case 'g':
			q.queryTable.Select(1, 0)
			return nil
		case 'G':
			q.queryTable.Select(q.queryTable.GetRowCount()-1, 0)
			return nil
		case 'd':
			q.deleteSelected()
			return nil
		case 'r':
			q.runSelected()
			return nil
		}

		if event.Key() == tcell.KeyEnter {
			q.openSelected()
			return nil
		}

		return event
	})

	return q
}

func (q *QueryList) Name() string { return "Saved Queries" }

func (q *QueryList) Start() {
	q.MasterDetailView.Start()
	q.loadQueries()
}

func (q *QueryList) Stop() {
	q.MasterDetailView.Stop()
}

func (q *QueryList) loadQueries() {
	profileName := q.app.ActiveProfileName()
	profile, ok := q.app.Config().GetProfile(profileName)
	if !ok {
		return
	}
	q.queries = profile.SavedQueries
	q.applyFilter()
}

func (q *QueryList) applyFilter() {
	if q.searchQuery == "" {
		q.filtered = q.queries
	} else {
		search := strings.ToLower(q.searchQuery)
		q.filtered = nil
		for _, sq := range q.queries {
			if strings.Contains(strings.ToLower(sq.Name), search) || strings.Contains(strings.ToLower(sq.Query), search) {
				q.filtered = append(q.filtered, sq)
			}
		}
	}
	q.renderQueries()
}

func (q *QueryList) renderQueries() {
	q.queryTable.Clear()

	headerStyle := tcell.StyleDefault.Bold(true).Foreground(theme.Get().Accent())
	q.queryTable.SetCell(0, 0, tview.NewTableCell("Name").SetStyle(headerStyle).SetSelectable(false))

	for i, sq := range q.filtered {
		q.queryTable.SetCell(i+1, 0, tview.NewTableCell(" "+sq.Name))
	}

	if len(q.filtered) > 0 {
		q.queryTable.Select(1, 0)
	}

	title := fmt.Sprintf("Saved Queries (%d)", len(q.filtered))
	if len(q.filtered) != len(q.queries) {
		title += fmt.Sprintf(" / %d total", len(q.queries))
	}
	q.MasterDetailView.SetMasterTitle(title)
}

func (q *QueryList) onSelectionChanged(row int) {
	if row <= 0 || row-1 >= len(q.filtered) {
		q.preview.SetText("")
		return
	}
	sq := q.filtered[row-1]
	q.preview.SetText(fmt.Sprintf("[::b]%s[::-]\n\n%s", sq.Name, sq.Query))
}

func (q *QueryList) selectedQuery() *config.SavedQuery {
	row, _ := q.queryTable.GetSelection()
	if row <= 0 || row-1 >= len(q.filtered) {
		return nil
	}
	return &q.filtered[row-1]
}

func (q *QueryList) openSelected() {
	sq := q.selectedQuery()
	if sq == nil {
		return
	}
	q.app.NavigateToQueryEditorWithSQL(sq.Query)
}

func (q *QueryList) runSelected() {
	sq := q.selectedQuery()
	if sq == nil {
		return
	}
	q.app.NavigateToQueryEditorWithSQL(sq.Query)
}

func (q *QueryList) deleteSelected() {
	sq := q.selectedQuery()
	if sq == nil {
		return
	}

	profileName := q.app.ActiveProfileName()
	q.app.Config().DeleteSavedQuery(profileName, sq.Name)
	go q.app.Config().Save()
	q.app.ShowSuccess(fmt.Sprintf("Deleted: %s", sq.Name))
	q.loadQueries()
}

var _ nav.Component = (*QueryList)(nil)
