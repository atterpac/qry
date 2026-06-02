package view

import (
	"fmt"
	"strings"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/config"
)

// QueryList is a MasterDetailView showing saved queries and SQL preview.
type QueryList struct {
	*components.MasterDetailView
	app         *App
	queryTable  *core.Table
	preview     *core.TextView
	queries     []config.SavedQuery
	filtered    []config.SavedQuery
	searchQuery string
}

func NewQueryList(app *App) *QueryList {
	q := &QueryList{
		app:        app,
		queryTable: core.NewTable(),
		preview:    core.NewTextView(),
	}

	q.queryTable.SetSelectable(true, false)
	q.queryTable.SetFixed(1, 0)
	q.queryTable.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))

	q.preview.SetDynamicColors(true)
	q.preview.SetWordWrap(true)

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

	return q
}

// HandleKey routes keyboard input: search keys first, then vim navigation and
// view actions, falling back to the table's own navigation (arrow keys).
func (q *QueryList) HandleKey(event *tcell.EventKey) bool {
	if q.MasterDetailView.HandleSearchKey(event) {
		return true
	}

	if event.Key() == tcell.KeyRune {
		switch event.Rune() {
		case 'j':
			row, _ := q.queryTable.GetSelection()
			if row < q.queryTable.GetRowCount()-1 {
				q.queryTable.Select(row+1, 0)
			}
			return true
		case 'k':
			row, _ := q.queryTable.GetSelection()
			if row > 1 {
				q.queryTable.Select(row-1, 0)
			}
			return true
		case 'g':
			q.queryTable.Select(1, 0)
			return true
		case 'G':
			q.queryTable.Select(q.queryTable.GetRowCount()-1, 0)
			return true
		case 'd':
			q.deleteSelected()
			return true
		case 'r':
			q.runSelected()
			return true
		}
	}

	if event.Key() == tcell.KeyEnter {
		q.openSelected()
		return true
	}

	return q.queryTable.HandleKey(event)
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

	q.queryTable.SetCell(0, 0, core.NewTableCell("Name").
		SetTextColor(theme.Get().Accent()).
		SetSelectable(false))

	for i, sq := range q.filtered {
		q.queryTable.SetCell(i+1, 0, core.NewTableCell(" "+sq.Name))
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
