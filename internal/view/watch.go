package view

import (
	"context"
	"fmt"
	"time"

	"github.com/atterpac/dado/theme"
)

// WatchState holds the state for live watch mode.
type WatchState struct {
	interval time.Duration
	sql      string
	cancel   context.CancelFunc
	lastCols []string
	lastRows [][]string
	nextTick time.Time
	running  bool
}

// DiffResult represents the diff between two result sets.
type DiffResult struct {
	AddedRows    []int           // row indices in new data that are new
	RemovedRows  []int           // row indices in old data that were removed
	ChangedCells map[[2]int]bool // [row, col] → changed
}

// DiffByPosition compares two result sets positionally (row by row).
func DiffByPosition(oldCols, newCols []string, oldRows, newRows [][]string) *DiffResult {
	diff := &DiffResult{
		ChangedCells: make(map[[2]int]bool),
	}

	minRows := len(oldRows)
	if len(newRows) < minRows {
		minRows = len(newRows)
	}

	// Compare existing rows
	for r := 0; r < minRows; r++ {
		oldRow := oldRows[r]
		newRow := newRows[r]
		minCols := len(oldRow)
		if len(newRow) < minCols {
			minCols = len(newRow)
		}
		for c := 0; c < minCols; c++ {
			if oldRow[c] != newRow[c] {
				diff.ChangedCells[[2]int{r, c}] = true
			}
		}
	}

	// Added rows
	for r := len(oldRows); r < len(newRows); r++ {
		diff.AddedRows = append(diff.AddedRows, r)
	}

	// Removed rows
	for r := len(newRows); r < len(oldRows); r++ {
		diff.RemovedRows = append(diff.RemovedRows, r)
	}

	return diff
}

func (q *QueryEditor) startWatch(dur time.Duration) {
	if q.watch != nil && q.watch.running {
		q.stopWatch()
	}

	sql := q.editor.GetText()
	if sql == "" {
		q.app.ShowWarning("No query to watch")
		return
	}

	if dur < time.Second {
		dur = time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	q.watch = &WatchState{
		interval: dur,
		sql:      sql,
		cancel:   cancel,
		running:  true,
	}

	// Store initial result
	q.watch.lastCols = q.lastResultCols
	q.watch.lastRows = q.lastResultRows

	q.app.ShowInfo(fmt.Sprintf("Watching every %s (Esc to stop)", dur))

	go q.watchLoop(ctx)
}

func (q *QueryEditor) stopWatch() {
	if q.watch == nil || !q.watch.running {
		return
	}
	q.watch.cancel()
	q.watch.running = false
	q.watch = nil
	q.app.QueueUpdateDraw(func() {
		q.updateWatchStatus("")
		q.app.ShowInfo("Watch stopped")
	})
}

func (q *QueryEditor) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(q.watch.interval)
	defer ticker.Stop()

	// Countdown timer for status bar
	countdownTicker := time.NewTicker(time.Second)
	defer countdownTicker.Stop()

	q.watch.nextTick = time.Now().Add(q.watch.interval)

	for {
		select {
		case <-ctx.Done():
			return

		case <-countdownTicker.C:
			if q.watch == nil || !q.watch.running {
				return
			}
			remaining := time.Until(q.watch.nextTick)
			if remaining < 0 {
				remaining = 0
			}
			q.app.QueueUpdateDraw(func() {
				rowCount := len(q.lastResultRows)
				q.updateWatchStatus(fmt.Sprintf(
					"[%s]Watching: %s | Next in %ds | %d rows[-]",
					theme.TagAccent(), q.watch.interval, int(remaining.Seconds()), rowCount,
				))
			})

		case <-ticker.C:
			if q.watch == nil || !q.watch.running {
				return
			}
			q.watch.nextTick = time.Now().Add(q.watch.interval)
			q.executeWatchQuery(ctx)
		}
	}
}

func (q *QueryEditor) executeWatchQuery(ctx context.Context) {
	provider := q.app.Provider()
	if provider == nil {
		q.app.QueueUpdateDraw(func() {
			q.app.ShowError("Watch: not connected")
		})
		return
	}

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := provider.ExecuteQuery(queryCtx, q.watch.sql)
	if err != nil {
		q.app.QueueUpdateDraw(func() {
			q.app.ShowWarning(fmt.Sprintf("Watch query error: %v", err))
		})
		return
	}

	if len(result.Columns) == 0 {
		return
	}

	// Cap at 1000 rows
	rows := result.Rows
	if len(rows) > 1000 {
		rows = rows[:1000]
		q.app.QueueUpdateDraw(func() {
			q.app.ShowWarning("Watch: capped at 1000 rows")
		})
	}

	// Compute diff against previous results
	var diff *DiffResult
	if q.watch.lastRows != nil {
		diff = DiffByPosition(q.watch.lastCols, result.Columns, q.watch.lastRows, rows)
	}

	// Update stored results
	q.watch.lastCols = result.Columns
	q.watch.lastRows = rows

	// Apply diff highlighting by modifying display data
	displayRows := make([][]string, len(rows))
	for i, row := range rows {
		displayRows[i] = make([]string, len(row))
		copy(displayRows[i], row)
	}

	if diff != nil {
		// Color added rows green
		for _, r := range diff.AddedRows {
			if r < len(displayRows) {
				for c := range displayRows[r] {
					displayRows[r][c] = "[green]" + displayRows[r][c] + "[-]"
				}
			}
		}
		// Color changed cells yellow
		for pos := range diff.ChangedCells {
			r, c := pos[0], pos[1]
			if r < len(displayRows) && c < len(displayRows[r]) {
				displayRows[r][c] = "[yellow]" + displayRows[r][c] + "[-]"
			}
		}
	}

	q.app.QueueUpdateDraw(func() {
		q.lastResultCols = result.Columns
		q.lastResultRows = rows

		q.showGrid()
		q.source.SetSliceData(result.Columns, displayRows)

		changes := 0
		if diff != nil {
			changes = len(diff.ChangedCells) + len(diff.AddedRows) + len(diff.RemovedRows)
		}
		q.statusBar.SetText(fmt.Sprintf(" [green]%d rows[-] [yellow]%d changes[-] [%s](watching)[-]",
			len(rows), changes, theme.TagFgDim()))
	})
}

func (q *QueryEditor) updateWatchStatus(text string) {
	if text == "" {
		q.statusBar.SetText(fmt.Sprintf(" [%s]Ready[-]", theme.TagFgDim()))
	} else {
		q.statusBar.SetText(" " + text)
	}
}
