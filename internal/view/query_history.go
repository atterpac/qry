package view

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"
)

// HistoryEntry represents a single query execution record.
type HistoryEntry struct {
	Query    string
	Duration time.Duration
	Time     time.Time
	RowCount int
	Error    string
}

// QueryHistoryStore is a shared, session-scoped store for query history.
type QueryHistoryStore struct {
	mu      sync.RWMutex
	entries []HistoryEntry
	maxSize int
}

// NewQueryHistoryStore creates a history store with a max size.
func NewQueryHistoryStore(maxSize int) *QueryHistoryStore {
	return &QueryHistoryStore{
		maxSize: maxSize,
	}
}

// Add records a query execution.
func (s *QueryHistoryStore) Add(entry HistoryEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append([]HistoryEntry{entry}, s.entries...)
	if len(s.entries) > s.maxSize {
		s.entries = s.entries[:s.maxSize]
	}
}

// Entries returns a copy of all entries.
func (s *QueryHistoryStore) Entries() []HistoryEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]HistoryEntry, len(s.entries))
	copy(result, s.entries)
	return result
}

// QueryHistory is a MasterDetailView showing recent query executions.
type QueryHistory struct {
	*components.MasterDetailView
	app          *App
	historyTable *core.Table
	preview      *core.TextView
	entries      []HistoryEntry
	filtered     []HistoryEntry
	searchQuery  string
}

func NewQueryHistory(app *App) *QueryHistory {
	q := &QueryHistory{
		app:          app,
		historyTable: core.NewTable(),
		preview:      core.NewTextView(),
	}

	q.historyTable.SetSelectable(true, false)
	q.historyTable.SetFixed(1, 0)
	q.historyTable.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))

	q.preview.SetDynamicColors(true)
	q.preview.SetWordWrap(true)

	q.MasterDetailView = components.NewMasterDetailView().
		SetMasterTitle("Query History").
		SetDetailTitle("SQL").
		SetMasterContent(q.historyTable).
		SetDetailContent(q.preview).
		SetRatio(0.5).
		SetResizable(true)

	q.MasterDetailView.ConfigureEmpty("󰋚", "No History", "Execute queries to see them here")

	q.MasterDetailView.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Open in editor"},
		{Key: "s", Description: "Save as named query"},
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

	q.historyTable.SetSelectionChangedFunc(func(row, col int) {
		q.onSelectionChanged(row)
	})

	return q
}

func (q *QueryHistory) Name() string { return "Query History" }

func (q *QueryHistory) Start() {
	q.MasterDetailView.Start()
	q.loadHistory()
}

func (q *QueryHistory) Stop() {
	q.MasterDetailView.Stop()
}

// HandleKey routes keyboard events. Returns true if the key was consumed.
func (q *QueryHistory) HandleKey(ev *tcell.EventKey) bool {
	if q.MasterDetailView.HandleSearchKey(ev) {
		return true
	}

	switch ev.Rune() {
	case 'j':
		row, _ := q.historyTable.GetSelection()
		if row < q.historyTable.GetRowCount()-1 {
			q.historyTable.Select(row+1, 0)
		}
		return true
	case 'k':
		row, _ := q.historyTable.GetSelection()
		if row > 1 {
			q.historyTable.Select(row-1, 0)
		}
		return true
	case 'g':
		q.historyTable.Select(1, 0)
		return true
	case 'G':
		q.historyTable.Select(q.historyTable.GetRowCount()-1, 0)
		return true
	case 's':
		q.saveSelected()
		return true
	}

	if ev.Key() == tcell.KeyEnter {
		q.openSelected()
		return true
	}

	return q.historyTable.HandleKey(ev)
}

func (q *QueryHistory) loadHistory() {
	if q.app.history == nil {
		return
	}
	q.entries = q.app.history.Entries()
	q.applyFilter()
}

func (q *QueryHistory) applyFilter() {
	if q.searchQuery == "" {
		q.filtered = q.entries
	} else {
		search := strings.ToLower(q.searchQuery)
		q.filtered = nil
		for _, e := range q.entries {
			if strings.Contains(strings.ToLower(e.Query), search) {
				q.filtered = append(q.filtered, e)
			}
		}
	}
	q.renderHistory()
}

func (q *QueryHistory) renderHistory() {
	q.historyTable.Clear()

	accentColor := theme.Accent()
	q.historyTable.SetCell(0, 0, core.NewTableCell("Time").SetTextColor(accentColor).SetSelectable(false))
	q.historyTable.SetCell(0, 1, core.NewTableCell("Duration").SetTextColor(accentColor).SetSelectable(false))
	q.historyTable.SetCell(0, 2, core.NewTableCell("Query").SetTextColor(accentColor).SetSelectable(false))

	for i, entry := range q.filtered {
		timeStr := entry.Time.Format("15:04:05")
		durStr := entry.Duration.Truncate(time.Millisecond).String()
		queryPreview := truncate(entry.Query, 60)

		statusColor := tcell.ColorGreen
		if entry.Error != "" {
			statusColor = tcell.ColorRed
		}

		q.historyTable.SetCell(i+1, 0, core.NewTableCell(timeStr).SetTextColor(tcell.ColorGray))
		q.historyTable.SetCell(i+1, 1, core.NewTableCell(durStr).SetTextColor(statusColor))
		q.historyTable.SetCell(i+1, 2, core.NewTableCell(queryPreview))
	}

	if len(q.filtered) > 0 {
		q.historyTable.Select(1, 0)
	}

	title := fmt.Sprintf("Query History (%d)", len(q.filtered))
	if len(q.filtered) != len(q.entries) {
		title += fmt.Sprintf(" / %d total", len(q.entries))
	}
	q.MasterDetailView.SetMasterTitle(title)
}

func (q *QueryHistory) onSelectionChanged(row int) {
	if row <= 0 || row-1 >= len(q.filtered) {
		q.preview.SetText("")
		return
	}

	entry := q.filtered[row-1]
	text := fmt.Sprintf("[::b]Executed[::-] %s\n[::b]Duration[::-] %s\n",
		entry.Time.Format("2006-01-02 15:04:05"),
		entry.Duration.Truncate(time.Millisecond))

	if entry.Error != "" {
		text += fmt.Sprintf("[%s]Error: %s[-]\n", theme.TagError(), entry.Error)
	} else {
		text += fmt.Sprintf("[%s]%d rows[-]\n", theme.TagSuccess(), entry.RowCount)
	}

	text += fmt.Sprintf("\n%s", entry.Query)
	q.preview.SetText(text)
}

func (q *QueryHistory) selectedEntry() *HistoryEntry {
	row, _ := q.historyTable.GetSelection()
	if row <= 0 || row-1 >= len(q.filtered) {
		return nil
	}
	return &q.filtered[row-1]
}

func (q *QueryHistory) openSelected() {
	entry := q.selectedEntry()
	if entry == nil {
		return
	}
	q.app.NavigateToQueryEditorWithSQL(entry.Query)
}

func (q *QueryHistory) saveSelected() {
	entry := q.selectedEntry()
	if entry == nil {
		return
	}

	input := components.NewTextField("query-name")
	input.SetLabel("Query name: ")
	input.SetPlaceholder("Enter a name...")
	input.SetOnSubmit(func(ev *components.SubmitEvent) {
		name := input.GetValue()
		if name != "" {
			profileName := q.app.ActiveProfileName()
			q.app.Config().SavedQueryForProfile(profileName, name, entry.Query)
			go q.app.Config().Save()
			q.app.ShowSuccess(fmt.Sprintf("Saved: %s", name))
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

var _ nav.Component = (*QueryHistory)(nil)
