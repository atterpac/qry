package view

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/engine"
)

// SchemaDiff displays a side-by-side schema comparison between two profiles.
type SchemaDiff struct {
	*components.MasterDetailView
	app           *App
	sourceProfile string
	targetProfile string
	schema        string
	tableList     *core.Table
	diffViewer    *components.DiffViewer
	diffResult    *engine.SchemaDiffResult
}

func NewSchemaDiff(app *App, targetProfile, schema string) *SchemaDiff {
	s := &SchemaDiff{
		app:           app,
		sourceProfile: app.ActiveProfileName(),
		targetProfile: targetProfile,
		schema:        schema,
	}

	s.tableList = core.NewTable()
	s.tableList.SetSelectable(true, false)
	s.tableList.SetFixed(1, 0)
	s.tableList.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))

	s.diffViewer = components.NewDiffViewer().
		SetShowLineNumbers(true).
		SetWordDiff(true)

	s.MasterDetailView = components.NewMasterDetailView().
		SetMasterTitle("Tables").
		SetDetailTitle("Column Diff").
		SetMasterContent(s.tableList).
		SetDetailContent(s.diffViewer).
		SetRatio(0.3).
		SetResizable(true)

	s.MasterDetailView.ConfigureEmpty("󰆧", "Schema Diff", "Loading...")

	s.tableList.SetSelectionChangedFunc(func(row, col int) {
		if row < 1 || s.diffResult == nil {
			return
		}
		idx := row - 1 // account for header
		if idx < len(s.diffResult.Tables) {
			td := s.diffResult.Tables[idx]
			diffText := engine.GenerateColumnDiffText(td)
			s.diffViewer.SetUnifiedDiff(diffText)
		}
	})

	return s
}

func (s *SchemaDiff) Name() string {
	return fmt.Sprintf("Diff: %s vs %s", s.sourceProfile, s.targetProfile)
}

func (s *SchemaDiff) Start() {
	s.loadDiff()
}

func (s *SchemaDiff) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "j/k", Description: "Navigate tables"},
		{Key: "Tab", Description: "Switch pane"},
		{Key: "q/Esc", Description: "Back"},
	}
}

// HandleKey routes keyboard events to the table list. Returns true if consumed.
func (s *SchemaDiff) HandleKey(ev *tcell.EventKey) bool {
	return s.tableList.HandleKey(ev)
}

func (s *SchemaDiff) loadDiff() {
	go func() {
		// Get source schema from current provider
		sourceProvider := s.app.Provider()
		if sourceProvider == nil {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError("Source not connected")
			})
			return
		}

		// Create temporary target provider
		targetCfg, ok := s.app.Config().GetProfile(s.targetProfile)
		if !ok {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Profile %q not found", s.targetProfile))
			})
			return
		}

		targetCfg = targetCfg.ExpandEnv()
		targetProvider, err := engine.NewProvider(targetCfg.Engine)
		if err != nil {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Target engine error: %v", err))
			})
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := targetProvider.Connect(ctx, targetCfg); err != nil {
			cancel()
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Target connection failed: %v", err))
			})
			return
		}
		cancel()
		defer targetProvider.Close()

		// Determine schema
		schema := s.schema
		if schema == "" {
			schema = "public"
		}

		// Fetch tables from both concurrently
		type tableResult struct {
			tables map[string][]engine.ColumnInfo
			err    error
		}

		var wg sync.WaitGroup
		sourceCh := make(chan tableResult, 1)
		targetCh := make(chan tableResult, 1)

		fetchSchema := func(provider engine.Provider, ch chan<- tableResult) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			tables, err := provider.ListTables(ctx, schema)
			if err != nil {
				ch <- tableResult{err: err}
				return
			}

			result := make(map[string][]engine.ColumnInfo)
			sem := make(chan struct{}, 5) // concurrency limiter

			var mu sync.Mutex
			var fetchWg sync.WaitGroup

			for _, t := range tables {
				fetchWg.Add(1)
				go func(tbl engine.TableInfo) {
					defer fetchWg.Done()
					sem <- struct{}{}
					defer func() { <-sem }()

					cols, err := provider.DescribeTable(ctx, schema, tbl.Name)
					if err != nil {
						return
					}
					mu.Lock()
					result[tbl.Name] = cols
					mu.Unlock()
				}(t)
			}
			fetchWg.Wait()

			ch <- tableResult{tables: result}
		}

		wg.Add(2)
		go fetchSchema(sourceProvider, sourceCh)
		go fetchSchema(targetProvider, targetCh)
		wg.Wait()

		sourceResult := <-sourceCh
		targetResult := <-targetCh

		if sourceResult.err != nil {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Source schema error: %v", sourceResult.err))
			})
			return
		}
		if targetResult.err != nil {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Target schema error: %v", targetResult.err))
			})
			return
		}

		diff := engine.ComputeSchemaDiff(sourceResult.tables, targetResult.tables)

		s.app.QueueUpdateDraw(func() {
			s.diffResult = &diff
			s.renderTableList()

			if len(diff.Tables) == 0 {
				s.MasterDetailView.ConfigureEmpty("✓", "No Differences", "Schemas are identical")
				s.app.ShowSuccess("Schemas are identical")
			} else {
				s.app.ShowInfo(fmt.Sprintf("Found %d table differences", len(diff.Tables)))
			}
		})
	}()
}

func (s *SchemaDiff) renderTableList() {
	s.tableList.Clear()

	// Header
	s.tableList.SetCell(0, 0, core.NewTableCell(" Status ").SetSelectable(false).SetTextColor(tcell.ColorYellow))
	s.tableList.SetCell(0, 1, core.NewTableCell(" Table ").SetSelectable(false).SetTextColor(tcell.ColorYellow))
	s.tableList.SetCell(0, 2, core.NewTableCell(" Changes ").SetSelectable(false).SetTextColor(tcell.ColorYellow))

	if s.diffResult == nil {
		return
	}

	for i, td := range s.diffResult.Tables {
		row := i + 1

		var statusIcon string
		var statusColor tcell.Color
		switch td.Status {
		case "added":
			statusIcon = "+ "
			statusColor = tcell.ColorGreen
		case "removed":
			statusIcon = "- "
			statusColor = tcell.ColorRed
		case "modified":
			statusIcon = "~ "
			statusColor = tcell.ColorYellow
		}

		s.tableList.SetCell(row, 0, core.NewTableCell(statusIcon).SetTextColor(statusColor))
		s.tableList.SetCell(row, 1, core.NewTableCell(td.Name).SetTextColor(statusColor))
		s.tableList.SetCell(row, 2, core.NewTableCell(fmt.Sprintf("%d", len(td.ColumnDiffs))).SetTextColor(statusColor))
	}
}

var _ nav.Component = (*SchemaDiff)(nil)
