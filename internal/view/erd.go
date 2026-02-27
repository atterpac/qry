package view

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
)

// ErdView displays an ERD (Entity Relationship Diagram) for the current schema.
type ErdView struct {
	erdGraph *components.ERDGraph
	app      *App
	schema   string

	mu      sync.Mutex
	loading bool
}

// NewErdView creates a new ErdView for the given schema.
func NewErdView(app *App, schema string) *ErdView {
	v := &ErdView{
		erdGraph: components.NewERDGraph(),
		app:      app,
		schema:   schema,
	}

	v.erdGraph.SetOnSelect(func(t *components.ERDTable) {
		if t != nil {
			v.app.NavigateToTableData(v.schema, t.Name)
		}
	})

	return v
}

func (v *ErdView) Name() string { return "ERD" }

func (v *ErdView) Start() {
	v.loadSchema()
}

func (v *ErdView) Stop() {}

func (v *ErdView) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "hjkl", Description: "Jump node"},
		{Key: "Tab", Description: "Cycle nodes"},
		{Key: "Enter", Description: "Open table"},
		{Key: "/", Description: "Search table"},
		{Key: "c", Description: "Center"},
		{Key: "Arrows", Description: "Pan"},
	}
}

func (v *ErdView) loadSchema() {
	v.mu.Lock()
	if v.loading {
		v.mu.Unlock()
		return
	}
	v.loading = true
	v.mu.Unlock()

	provider := v.app.Provider()
	if provider == nil {
		v.mu.Lock()
		v.loading = false
		v.mu.Unlock()
		return
	}

	go func() {
		defer func() {
			v.mu.Lock()
			v.loading = false
			v.mu.Unlock()
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		tables, err := provider.ListTables(ctx, v.schema)
		if err != nil {
			v.app.QueueUpdateDraw(func() {
				v.app.ShowError("ERD: failed to list tables: " + err.Error())
			})
			return
		}

		if len(tables) == 0 {
			v.app.QueueUpdateDraw(func() {
				v.app.ShowInfo("ERD: no tables found")
			})
			return
		}

		type tableResult struct {
			id      string
			table   *components.ERDTable
			rels    []*components.ERDRelation
			errDesc error
			errFK   error
		}

		results := make([]tableResult, len(tables))

		var wg sync.WaitGroup
		sem := make(chan struct{}, 8) // limit concurrency

		for i, ti := range tables {
			wg.Add(1)
			go func(idx int, name, schema string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
				defer tcancel()

				cols, errDesc := provider.DescribeTable(tctx, schema, name)
				fks, errFK := provider.GetForeignKeys(tctx, schema, name)

				res := tableResult{
					id:      name,
					errDesc: errDesc,
					errFK:   errFK,
				}

				erdCols := make([]components.ERDColumn, 0, len(cols))
				for _, c := range cols {
					erdCols = append(erdCols, components.ERDColumn{
						Name: c.Name,
						Type: c.DataType,
						IsPK: c.IsPrimaryKey,
					})
				}

				// Mark FK columns and build relations.
				var rels []*components.ERDRelation
				for _, fk := range fks {
					if fk.IsInbound {
						continue // only outbound to avoid duplication
					}
					// Mark the column as FK.
					for j := range erdCols {
						if erdCols[j].Name == fk.Column {
							erdCols[j].IsFK = true
							erdCols[j].FKTarget = fk.ReferencedTable + "." + fk.ReferencedColumn
							break
						}
					}
					rels = append(rels, &components.ERDRelation{
						FromTable:   name,
						FromColumn:  fk.Column,
						ToTable:     fk.ReferencedTable,
						ToColumn:    fk.ReferencedColumn,
						Cardinality: components.OneToMany,
						Type:        components.ERDSolid,
					})
				}

				res.table = &components.ERDTable{
					ID:      name,
					Name:    name,
					Columns: erdCols,
				}
				res.rels = rels
				results[idx] = res
			}(i, ti.Name, ti.Schema)
		}

		wg.Wait()

		// Build final data.
		erdTables := make([]*components.ERDTable, 0, len(results))
		var erdRelations []*components.ERDRelation

		// Collect all table IDs for cross-reference validation.
		tableIDs := make(map[string]bool, len(results))
		for _, r := range results {
			if r.table != nil {
				tableIDs[r.table.ID] = true
			}
		}

		for _, r := range results {
			if r.table != nil {
				erdTables = append(erdTables, r.table)
			}
			for _, rel := range r.rels {
				// Only include relations where target table is in the graph.
				if tableIDs[rel.ToTable] {
					erdRelations = append(erdRelations, rel)
				}
			}
		}

		v.app.QueueUpdateDraw(func() {
			v.erdGraph.SetData(erdTables, erdRelations)
		})
	}()
}

// tview.Primitive delegation

func (v *ErdView) Draw(screen tcell.Screen)        { v.erdGraph.Draw(screen) }
func (v *ErdView) GetRect() (int, int, int, int)    { return v.erdGraph.GetRect() }
func (v *ErdView) SetRect(x, y, width, height int)  { v.erdGraph.SetRect(x, y, width, height) }
func (v *ErdView) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		if event.Key() == tcell.KeyRune && event.Rune() == '/' {
			v.showSearch()
			return
		}
		if handler := v.erdGraph.InputHandler(); handler != nil {
			handler(event, setFocus)
		}
	}
}

func (v *ErdView) showSearch() {
	searchFn := func(query string) {
		if query == "" {
			return
		}
		q := strings.ToLower(query)
		for _, id := range v.erdGraph.TableOrder() {
			if strings.Contains(strings.ToLower(id), q) {
				v.erdGraph.SetFocusedTable(id)
				return
			}
		}
	}

	v.app.ShowSearchMode("", components.SearchCallbacks{
		OnChange: searchFn,
		OnSubmit: func(text string) {
			searchFn(text)
		},
		OnCancel: func() {},
	})
}
func (v *ErdView) Focus(delegate func(tview.Primitive)) { v.erdGraph.Focus(delegate) }
func (v *ErdView) Blur()                                { v.erdGraph.Blur() }
func (v *ErdView) HasFocus() bool                       { return v.erdGraph.HasFocus() }
func (v *ErdView) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return v.erdGraph.MouseHandler()
}
func (v *ErdView) PasteHandler() func(string, func(tview.Primitive)) { return nil }

var _ nav.Component = (*ErdView)(nil)
