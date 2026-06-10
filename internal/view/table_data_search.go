package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/async"

	"github.com/atterpac/qry/internal/engine"
)

func (t *TableData) nextPage() {
	t.offset += t.limit
	t.loadData()
}

func (t *TableData) prevPage() {
	t.offset -= t.limit
	if t.offset < 0 {
		t.offset = 0
	}
	t.loadData()
}

// searchData runs a filtered query reusing cached column info.
// It cancels any previous in-flight search query.
func (t *TableData) searchData() {
	provider := t.app.Provider()
	if provider == nil || len(t.columns) == 0 {
		return
	}

	// Cancel any in-flight search
	if t.searchCancel != nil {
		t.searchCancel()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.searchCancel = cancel

	go func() {
		defer cancel()

		tableName := provider.QuoteIdentifier(t.table)
		if t.schema != "" {
			caps := provider.Capabilities()
			if caps.HasSchemas {
				tableName = provider.QuoteIdentifier(t.schema) + "." + tableName
			}
		}

		query := fmt.Sprintf("SELECT * FROM %s", tableName)
		if t.searchActive && t.searchFilter != "" {
			var colNames []string
			for _, c := range t.columns {
				colNames = append(colNames, c.Name)
			}
			filters := engine.ParseSearchInput(t.searchFilter, colNames)
			if clause := provider.BuildSearchClause(colNames, filters); clause != "" {
				query += " WHERE " + clause
			}
		}
		if t.sortColumn != "" {
			query += fmt.Sprintf(" ORDER BY %s %s", provider.QuoteIdentifier(t.sortColumn), t.sortDir)
		}
		query += fmt.Sprintf(" LIMIT %d OFFSET %d", t.limit, t.offset)

		result, err := provider.ExecuteQuery(ctx, query)
		if err != nil {
			if ctx.Err() != nil {
				return // cancelled, ignore
			}
			t.app.QueueUpdateDraw(func() {
				t.app.ShowError(fmt.Sprintf("Query failed: %v", err))
			})
			return
		}

		t.app.QueueUpdateDraw(func() {
			t.setGridData(result)
		})
	}()
}

func (t *TableData) showSearchBar() {
	t.app.statusBar.SetCommandPrompt("/ ")
	t.app.statusBar.SetCommandPlaceholder("search... (col:value or term)")

	var colNames []string
	for _, c := range t.columns {
		colNames = append(colNames, c.Name)
	}
	t.app.statusBar.SetOnComplete(func(input string) []string {
		// Offer column name completions with : suffix
		var matches []string
		// Get the last token being typed
		parts := strings.Fields(input)
		var prefix string
		if len(parts) > 0 {
			prefix = parts[len(parts)-1]
		}
		for _, col := range colNames {
			candidate := col + ":"
			if prefix == "" || strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(prefix)) {
				matches = append(matches, candidate)
			}
		}
		return matches
	})

	t.app.statusBar.EnterCommandMode()
	t.app.app.SetFocus(t.app.statusBar)

	t.app.statusBar.SetOnCommandSubmit(func(text string) {
		t.app.statusBar.ExitCommandMode()
		text = strings.TrimSpace(text)
		if text != "" {
			t.searchFilter = text
			t.searchActive = true
			t.app.gridSearching = true
			t.offset = 0
			t.searchData()
		} else {
			t.searchFilter = ""
			t.searchActive = false
			t.app.gridSearching = false
			t.offset = 0
			t.loadData()
		}
		t.app.refocusCurrent()
	})
	t.app.statusBar.SetOnCommandCancel(func() {
		if t.searchCancel != nil {
			t.searchCancel()
		}
		t.app.statusBar.ExitCommandMode()
		// Restore to unfiltered state
		t.searchFilter = ""
		t.searchActive = false
		t.app.gridSearching = false
		t.offset = 0
		t.loadData()
		t.app.refocusCurrent()
	})
}

// SetFilter configures a pre-applied search filter for this view.
// When used for FK navigation, the filter is treated as part of the navigation
// so Escape pops back rather than clearing the filter.
func (t *TableData) SetFilter(filter string) {
	t.searchFilter = filter
	t.searchActive = true
	t.filterFromNav = true
}

func (t *TableData) SetSort(col, dir string) {
	t.sortColumn = col
	t.sortDir = dir
	t.offset = 0
	t.loadData()
}

func (t *TableData) runCount() {
	provider := t.app.Provider()
	if provider == nil {
		return
	}

	async.NewLoader[string]().
		WithTimeout(30 * time.Second).
		OnSuccess(func(count string) {
			t.app.ShowInfo(fmt.Sprintf("Total rows: %s", count))
		}).
		OnError(func(err error) {
			t.app.ShowError(fmt.Sprintf("Count failed: %v", err))
		}).
		Run(func(ctx context.Context) (string, error) {
			tableName := provider.QuoteIdentifier(t.table)
			if t.schema != "" {
				caps := provider.Capabilities()
				if caps.HasSchemas {
					tableName = provider.QuoteIdentifier(t.schema) + "." + tableName
				}
			}

			query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
			if t.searchActive && t.searchFilter != "" {
				var colNames []string
				for _, c := range t.columns {
					colNames = append(colNames, c.Name)
				}
				filters := engine.ParseSearchInput(t.searchFilter, colNames)
				if clause := provider.BuildSearchClause(colNames, filters); clause != "" {
					query += " WHERE " + clause
				}
			}

			result, err := provider.ExecuteQuery(ctx, query)
			if err != nil {
				return "", err
			}

			count := "?"
			if result != nil && len(result.Rows) > 0 && len(result.Rows[0]) > 0 {
				count = result.Rows[0][0]
			}
			return count, nil
		})
}
