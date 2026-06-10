package view

import (
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"
)

// DatabaseList shows a list of databases. Only useful for engines with HasDatabases.
type DatabaseList struct {
	*components.MasterDetailView
	app         *App
	dbTable     *components.Table
	detail      *core.TextView
	databases   []string
	filtered    []string
	searchQuery string
}

func NewDatabaseList(app *App) *DatabaseList {
	d := &DatabaseList{
		app:     app,
		dbTable: components.NewTable(),
		detail:  core.NewTextView(),
	}

	// components.Table tracks theme.Bg()/theme.SelectionStyle() at draw time.
	d.dbTable.SetSelectable(true, false)
	d.dbTable.SetFixed(1, 0)

	d.detail.SetDynamicColors(true)

	d.MasterDetailView = components.NewMasterDetailView().
		SetMasterTitle("Databases").
		SetDetailTitle("Info").
		SetMasterContent(d.dbTable).
		SetDetailContent(d.detail).
		SetRatio(0.4).
		SetResizable(true)

	d.MasterDetailView.ConfigureEmpty("󰆼", "No Databases", "This engine may not support multiple databases")

	d.MasterDetailView.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Select database"},
		{Key: "/", Description: "Search"},
		{Key: "r", Description: "Refresh"},
	})

	d.MasterDetailView.EnableSearch(func(currentText string, callbacks components.SearchCallbacks) {
		app.ShowSearchMode(currentText, callbacks)
	})
	d.MasterDetailView.SetOnSearch(func(query string) {
		d.searchQuery = query
		d.applyFilter()
	})
	d.MasterDetailView.SetOnSearchCancel(func() {
		d.searchQuery = ""
		d.applyFilter()
	})

	d.dbTable.SetSelectionChangedFunc(func(row, col int) {
		d.onSelectionChanged(row)
	})

	return d
}

// HandleKey routes key events for DatabaseList, handling vim navigation and custom actions.
func (d *DatabaseList) HandleKey(ev *tcell.EventKey) bool {
	if d.MasterDetailView.HandleSearchKey(ev) {
		return true
	}

	if ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'j':
			row, _ := d.dbTable.GetSelection()
			if row < d.dbTable.GetRowCount()-1 {
				d.dbTable.Select(row+1, 0)
			}
			return true
		case 'k':
			row, _ := d.dbTable.GetSelection()
			if row > 1 {
				d.dbTable.Select(row-1, 0)
			}
			return true
		case 'g':
			d.dbTable.Select(1, 0)
			return true
		case 'G':
			d.dbTable.Select(d.dbTable.GetRowCount()-1, 0)
			return true
		case 'r':
			d.loadDatabases()
			return true
		}
	}

	if ev.Key() == tcell.KeyEnter {
		row, _ := d.dbTable.GetSelection()
		if row > 0 && row-1 < len(d.filtered) {
			d.app.ShowInfo(fmt.Sprintf("Database: %s", d.filtered[row-1]))
		}
		return true
	}

	return d.MasterDetailView.HandleKey(ev)
}

func (d *DatabaseList) Name() string { return "Databases" }

func (d *DatabaseList) Start() {
	d.MasterDetailView.Start()
	d.loadDatabases()
}

func (d *DatabaseList) Stop() {
	d.MasterDetailView.Stop()
}

func (d *DatabaseList) loadDatabases() {
	provider := d.app.Provider()
	if provider == nil {
		return
	}

	if !provider.Capabilities().HasDatabases {
		d.app.ShowWarning("This engine does not support multiple databases")
		return
	}

	async.NewLoader[[]string]().
		WithTimeout(10 * time.Second).
		OnSuccess(func(databases []string) {
			d.databases = databases
			d.applyFilter()
		}).
		OnError(func(err error) {
			d.app.ShowError(fmt.Sprintf("Failed to list databases: %v", err))
		}).
		Run(provider.ListDatabases)
}

func (d *DatabaseList) applyFilter() {
	if d.searchQuery == "" {
		d.filtered = d.databases
	} else {
		search := strings.ToLower(d.searchQuery)
		d.filtered = nil
		for _, name := range d.databases {
			if strings.Contains(strings.ToLower(name), search) {
				d.filtered = append(d.filtered, name)
			}
		}
	}
	d.renderDatabases()
}

func (d *DatabaseList) renderDatabases() {
	d.dbTable.Clear()

	accentColor := theme.Get().Accent()
	d.dbTable.SetCell(0, 0, core.NewTableCell("Database").SetTextColor(accentColor).SetSelectable(false))

	for i, name := range d.filtered {
		d.dbTable.SetCell(i+1, 0, core.NewTableCell(" "+name))
	}

	if len(d.filtered) > 0 {
		d.dbTable.Select(1, 0)
	}

	title := fmt.Sprintf("Databases (%d)", len(d.filtered))
	if len(d.filtered) != len(d.databases) {
		title += fmt.Sprintf(" / %d total", len(d.databases))
	}
	d.MasterDetailView.SetMasterTitle(title)
}

func (d *DatabaseList) onSelectionChanged(row int) {
	if row <= 0 || row-1 >= len(d.filtered) {
		d.detail.SetText("")
		return
	}
	name := d.filtered[row-1]
	d.detail.SetText(fmt.Sprintf("[::b]%s[::-]", name))
}

var _ nav.Component = (*DatabaseList)(nil)
