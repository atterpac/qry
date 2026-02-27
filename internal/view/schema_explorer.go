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

	"github.com/atterpac/qry/internal/engine"
)

// SchemaExplorer is a MasterDetailView showing tables on the left and column info on the right.
type SchemaExplorer struct {
	*components.MasterDetailView
	app           *App
	tableList     *tview.Table
	detailView    *tview.TextView
	tables        []engine.TableInfo
	filtered      []engine.TableInfo
	schema        string
	schemas       []string
	schemaIdx     int
	searchQuery   string
}

func NewSchemaExplorer(app *App) *SchemaExplorer {
	s := &SchemaExplorer{
		app:        app,
		tableList:  tview.NewTable(),
		detailView: tview.NewTextView(),
		schema:     "public",
	}

	s.tableList.SetSelectable(true, false)
	s.tableList.SetFixed(1, 0)
	s.tableList.SetSelectedStyle(tcell.StyleDefault.
		Foreground(tcell.ColorWhite).
		Background(tcell.ColorDarkCyan))
	theme.Register(s.tableList)

	s.detailView.SetDynamicColors(true)
	s.detailView.SetWordWrap(true)
	theme.Register(s.detailView)

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

	s.tableList.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if s.MasterDetailView.HandleSearchKey(event) {
			return nil
		}

		switch event.Rune() {
		case 'j':
			row, _ := s.tableList.GetSelection()
			if row < s.tableList.GetRowCount()-1 {
				s.tableList.Select(row+1, 0)
			}
			return nil
		case 'k':
			row, _ := s.tableList.GetSelection()
			if row > 1 {
				s.tableList.Select(row-1, 0)
			}
			return nil
		case 'e':
			s.app.NavigateToQueryEditor()
			return nil
		case 'r':
			s.loadTables()
			return nil
		case 'g':
			s.tableList.Select(1, 0)
			return nil
		case 'G':
			s.tableList.Select(s.tableList.GetRowCount()-1, 0)
			return nil
		case 's':
			s.cycleSchema()
			return nil
		case 'i':
			s.app.NavigateToConnectionInfo()
			return nil
		case 'E':
			s.app.NavigateToERD(s.schema)
			return nil
		}

		if event.Key() == tcell.KeyEnter {
			row, _ := s.tableList.GetSelection()
			if row > 0 && row-1 < len(s.filtered) {
				t := s.filtered[row-1]
				s.app.NavigateToTableData(t.Schema, t.Name)
			}
			return nil
		}

		return event
	})

	return s
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
	headerStyle := tcell.StyleDefault.Bold(true).Foreground(theme.Get().Accent())
	s.tableList.SetCell(0, 0, tview.NewTableCell("Name").SetStyle(headerStyle).SetSelectable(false).SetExpansion(1))
	s.tableList.SetCell(0, 1, tview.NewTableCell("Type").SetStyle(headerStyle).SetSelectable(false))

	for i, t := range s.filtered {
		typeIcon := "󰓫"
		if t.Type == "view" {
			typeIcon = "󰈔"
		}
		s.tableList.SetCell(i+1, 0, tview.NewTableCell(t.Name).SetExpansion(1))
		s.tableList.SetCell(i+1, 1, tview.NewTableCell(typeIcon+" "+t.Type).SetTextColor(tcell.ColorGray))
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
				s.detailView.SetText(fmt.Sprintf("[red]Error: %v[-]", err))
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
			pkTag = " [yellow]PK[-]"
		}
		nullTag := ""
		if !col.Nullable {
			nullTag = " [red]NOT NULL[-]"
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
