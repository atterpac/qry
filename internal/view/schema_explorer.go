package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/engine"
)

// SchemaExplorer is a MasterDetailView showing tables on the left and column info on the right.
type SchemaExplorer struct {
	*components.MasterDetailView
	app         *App
	tableList   *components.Table
	detailView  *core.TextView
	tables      []engine.TableInfo
	filtered    []engine.TableInfo
	schema      string
	schemas     []string
	schemaIdx   int
	searchQuery string
}

func NewSchemaExplorer(app *App) *SchemaExplorer {
	s := &SchemaExplorer{
		app:        app,
		tableList:  components.NewTable(),
		detailView: core.NewTextView(),
		schema:     "public",
	}

	// components.Table re-applies theme.Bg()/theme.SelectionStyle() every draw
	// and manages its own theme subscription, so no colors are hardcoded here.
	s.tableList.SetSelectable(true, false)
	s.tableList.SetFixed(1, 0)

	s.detailView.SetDynamicColors(true)

	s.MasterDetailView = components.NewMasterDetailView().
		SetMasterTitle("Tables").
		SetDetailTitle("Columns").
		SetMasterContent(s.tableList).
		SetDetailContent(s.detailView).
		SetRatio(0.35).
		SetResizable(true)

	s.MasterDetailView.ConfigureEmpty("󰗃", "No Tables", "Connect to a database to browse tables")

	s.MasterDetailView.SetHints([]components.KeyHint{
		{Key: "Enter", Description: "Open table"},
		{Key: "/", Description: "Search"},
		{Key: "e", Description: "Query editor"},
		{Key: "E", Description: "ERD view"},
		{Key: "s", Description: "Switch schema"},
		{Key: "r", Description: "Refresh"},
	})

	// Enable search
	s.MasterDetailView.EnableSearch(func(currentText string, callbacks components.SearchCallbacks) {
		app.ShowSearchMode(currentText, callbacks)
	})
	s.MasterDetailView.SetOnSearch(func(query string) {
		s.searchQuery = query
		s.applyFilter()
	})
	s.MasterDetailView.SetOnSearchCancel(func() {
		s.searchQuery = ""
		s.applyFilter()
	})

	s.tableList.SetSelectionChangedFunc(func(row, col int) {
		s.onSelectionChanged(row)
	})

	return s
}

// HandleKey routes key events for SchemaExplorer, handling vim navigation and custom actions.
func (s *SchemaExplorer) HandleKey(ev *tcell.EventKey) bool {
	if s.MasterDetailView.HandleSearchKey(ev) {
		return true
	}

	if ev.Key() == tcell.KeyRune {
		switch ev.Rune() {
		case 'j':
			row, _ := s.tableList.GetSelection()
			if row < s.tableList.GetRowCount()-1 {
				s.tableList.Select(row+1, 0)
			}
			return true
		case 'k':
			row, _ := s.tableList.GetSelection()
			if row > 1 {
				s.tableList.Select(row-1, 0)
			}
			return true
		case 'e':
			s.app.NavigateToQueryEditor()
			return true
		case 'r':
			s.loadTables()
			return true
		case 'g':
			s.tableList.Select(1, 0)
			return true
		case 'G':
			s.tableList.Select(s.tableList.GetRowCount()-1, 0)
			return true
		case 's':
			s.cycleSchema()
			return true
		case 'i':
			s.app.NavigateToConnectionInfo()
			return true
		case 'E':
			s.app.NavigateToERD(s.schema)
			return true
		}
	}

	if ev.Key() == tcell.KeyEnter {
		row, _ := s.tableList.GetSelection()
		if row > 0 && row-1 < len(s.filtered) {
			t := s.filtered[row-1]
			s.app.NavigateToTableData(t.Schema, t.Name)
		}
		return true
	}

	return s.MasterDetailView.HandleKey(ev)
}

func (s *SchemaExplorer) Name() string { return "Tables" }

func (s *SchemaExplorer) Start() {
	s.MasterDetailView.Start()
	s.loadSchemas()
	s.loadTables()
}

func (s *SchemaExplorer) loadSchemas() {
	provider := s.app.Provider()
	if provider == nil {
		return
	}

	caps := provider.Capabilities()
	if !caps.HasSchemas {
		s.schema = ""
		s.schemas = nil
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		schemas, err := provider.ListSchemas(ctx, "")
		if err != nil {
			return
		}

		s.app.QueueUpdateDraw(func() {
			s.schemas = schemas
			// Find current schema index
			for i, name := range schemas {
				if name == s.schema {
					s.schemaIdx = i
					break
				}
			}
		})
	}()
}

func (s *SchemaExplorer) cycleSchema() {
	if len(s.schemas) <= 1 {
		s.app.ShowInfo("Only one schema available")
		return
	}
	s.schemaIdx = (s.schemaIdx + 1) % len(s.schemas)
	s.schema = s.schemas[s.schemaIdx]
	s.app.ShowInfo(fmt.Sprintf("Schema: %s", s.schema))
	s.loadTables()
}

func (s *SchemaExplorer) loadTables() {
	provider := s.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		tables, err := provider.ListTables(ctx, s.schema)
		if err != nil {
			s.app.QueueUpdateDraw(func() {
				s.app.ShowError(fmt.Sprintf("Failed to list tables: %v", err))
			})
			return
		}

		s.app.QueueUpdateDraw(func() {
			s.tables = tables
			s.applyFilter()
		})
	}()
}

func (s *SchemaExplorer) applyFilter() {
	if s.searchQuery == "" {
		s.filtered = s.tables
	} else {
		q := strings.ToLower(s.searchQuery)
		s.filtered = nil
		for _, t := range s.tables {
			if strings.Contains(strings.ToLower(t.Name), q) {
				s.filtered = append(s.filtered, t)
			}
		}
	}
	s.renderTableList()
}

func (s *SchemaExplorer) renderTableList() {
	s.tableList.Clear()

	// Header
	accentColor := theme.Get().Accent()
	s.tableList.SetCell(0, 0, core.NewTableCell("Name").SetTextColor(accentColor).SetSelectable(false).SetExpansion(1))
	s.tableList.SetCell(0, 1, core.NewTableCell("Type").SetTextColor(accentColor).SetSelectable(false))

	for i, t := range s.filtered {
		typeIcon := "󰓫"
		if t.Type == "view" {
			typeIcon = "󰈔"
		}
		s.tableList.SetCell(i+1, 0, core.NewTableCell(t.Name).SetExpansion(1))
		s.tableList.SetCell(i+1, 1, core.NewTableCell(typeIcon+" "+t.Type).SetTextColor(theme.FgMuted()))
	}

	if len(s.filtered) > 0 {
		s.tableList.Select(1, 0)
	}

	title := fmt.Sprintf("Tables (%d)", len(s.filtered))
	if s.schema != "" {
		title = fmt.Sprintf("%s — %s", s.schema, title)
	}
	if len(s.filtered) != len(s.tables) {
		title += fmt.Sprintf(" / %d total", len(s.tables))
	}
	s.MasterDetailView.SetMasterTitle(title)
}

func (s *SchemaExplorer) onSelectionChanged(row int) {
	if row <= 0 || row-1 >= len(s.filtered) {
		s.detailView.SetText("")
		return
	}

	table := s.filtered[row-1]
	provider := s.app.Provider()
	if provider == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		columns, err := provider.DescribeTable(ctx, table.Schema, table.Name)
		if err != nil {
			s.app.QueueUpdateDraw(func() {
				s.detailView.SetText(fmt.Sprintf("[%s]Error: %v[-]", theme.TagError(), err))
			})
			return
		}

		s.app.QueueUpdateDraw(func() {
			s.renderColumns(table, columns)
		})
	}()
}

func (s *SchemaExplorer) renderColumns(table engine.TableInfo, columns []engine.ColumnInfo) {
	var b strings.Builder

	fmt.Fprintf(&b, "[::b]%s[::-]\n", table.Name)
	if table.Schema != "" {
		fmt.Fprintf(&b, "[%s]Schema: %s[-]\n", theme.TagFgDim(), table.Schema)
	}
	fmt.Fprintf(&b, "[%s]Type: %s[-]\n\n", theme.TagFgDim(), table.Type)

	for _, col := range columns {
		pkTag := ""
		if col.IsPrimaryKey {
			pkTag = fmt.Sprintf(" [%s]PK[-]", theme.TagWarning())
		}
		nullTag := ""
		if !col.Nullable {
			nullTag = fmt.Sprintf(" [%s]NOT NULL[-]", theme.TagError())
		}
		defaultTag := ""
		if col.Default != "" {
			defaultTag = fmt.Sprintf(" [%s]DEFAULT %s[-]", theme.TagFgDim(), col.Default)
		}

		fmt.Fprintf(&b, "  [::b]%s[::-] [%s]%s[-]%s%s%s\n",
			col.Name, theme.TagFgDim(), col.DataType, pkTag, nullTag, defaultTag)
	}

	s.detailView.SetText(b.String())
	s.MasterDetailView.SetDetailTitle(fmt.Sprintf("Columns (%d)", len(columns)))
}

// CommandContext implements CommandContextProvider.
func (s *SchemaExplorer) CommandContext() CommandViewContext {
	ctx := CommandViewContext{
		Schema: s.schema,
	}
	if s.app.Provider() != nil {
		ctx.Engine = string(s.app.Provider().EngineType())
	}
	row, _ := s.tableList.GetSelection()
	if row > 0 && row-1 < len(s.filtered) {
		ctx.Table = s.filtered[row-1].Name
	}
	return ctx
}

var _ nav.Component = (*SchemaExplorer)(nil)
