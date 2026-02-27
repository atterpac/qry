package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// DatabaseList shows a list of databases. Only useful for engines with HasDatabases.
type DatabaseList struct {
	*components.MasterDetailView
	app         *App
	dbTable     *tview.Table
	detail      *tview.TextView
	databases   []string
	filtered    []string
	searchQuery string
}

func NewDatabaseList(app *App) *DatabaseList {
	d := &DatabaseList{
		app:    app,
		dbTable: tview.NewTable(),
		detail:  tview.NewTextView(),
	}

	d.dbTable.SetSelectable(true, false)
	d.dbTable.SetFixed(1, 0)
	d.dbTable.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))
	theme.Register(d.dbTable)

	d.detail.SetDynamicColors(true)
	d.detail.SetWordWrap(true)
	theme.Register(d.detail)

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

	d.dbTable.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if d.MasterDetailView.HandleSearchKey(event) {
			return nil
		}

		switch event.Rune() {
		case 'j':
			row, _ := d.dbTable.GetSelection()
			if row < d.dbTable.GetRowCount()-1 {
				d.dbTable.Select(row+1, 0)
			}
			return nil
		case 'k':
			row, _ := d.dbTable.GetSelection()
			if row > 1 {
				d.dbTable.Select(row-1, 0)
			}
			return nil
		case 'g':
			d.dbTable.Select(1, 0)
			return nil
		case 'G':
			d.dbTable.Select(d.dbTable.GetRowCount()-1, 0)
			return nil
		case 'r':
			d.loadDatabases()
			return nil
		}

		if event.Key() == tcell.KeyEnter {
			row, _ := d.dbTable.GetSelection()
			if row > 0 && row-1 < len(d.filtered) {
				d.app.ShowInfo(fmt.Sprintf("Database: %s", d.filtered[row-1]))
			}
			return nil
		}

		return event
	})

	return d
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

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		databases, err := provider.ListDatabases(ctx)
		if err != nil {
			d.app.QueueUpdateDraw(func() {
				d.app.ShowError(fmt.Sprintf("Failed to list databases: %v", err))
			})
			return
		}

		d.app.QueueUpdateDraw(func() {
			d.databases = databases
			d.applyFilter()
		})
	}()
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

	headerStyle := tcell.StyleDefault.Bold(true).Foreground(theme.Get().Accent())
	d.dbTable.SetCell(0, 0, tview.NewTableCell("Database").SetStyle(headerStyle).SetSelectable(false))

	for i, name := range d.filtered {
		d.dbTable.SetCell(i+1, 0, tview.NewTableCell(" "+name))
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
