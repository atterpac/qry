package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/nav"
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/engine"
)

// ExplainView displays a query execution plan as an interactive tree.
type ExplainView struct {
	*components.Split
	app         *App
	sql         string
	tree        *components.Tree
	detailPanel *components.Panel
	detailView  *core.TextView
	plan        *engine.PlanResult
}

func NewExplainView(app *App, sql string) *ExplainView {
	e := &ExplainView{
		app: app,
		sql: sql,
	}

	e.tree = components.NewTree().
		SetShowLines(true).
		SetShowIcons(true)

	e.detailView = core.NewTextView()
	e.detailView.SetDynamicColors(true)
	e.detailView.SetScrollable(true)
	e.detailView.SetWordWrap(true)
	e.detailPanel = components.NewPanel().
		SetTitle("Details").
		SetTitleAlign(components.TitleAlignLeft).
		SetContent(e.detailView)

	e.tree.SetOnHighlight(func(node *components.TreeNode) {
		if node != nil && node.Data != nil {
			if pn, ok := node.Data.(*engine.PlanNode); ok {
				e.updateDetail(pn)
			}
		}
	})

	e.tree.SetOnSelect(func(node *components.TreeNode) {
		if node != nil && node.Data != nil {
			if pn, ok := node.Data.(*engine.PlanNode); ok {
				e.updateDetail(pn)
			}
		}
	})

	e.Split = components.NewSplit().
		SetDirection(components.SplitHorizontal).
		SetTop(e.tree).
		SetBottom(e.detailPanel).
		SetRatio(0.6).
		SetResizable(true)

	return e
}

func (e *ExplainView) Name() string { return "Explain" }

// HandleKey intercepts 'r' (reload) and '/' (filter) before delegating to Split.
func (e *ExplainView) HandleKey(ev *tcell.EventKey) bool {
	switch ev.Rune() {
	case 'r':
		e.reload()
		return true
	case '/':
		e.showFilter()
		return true
	}
	return e.Split.HandleKey(ev)
}

func (e *ExplainView) Start() {
	e.loadPlan()
}

func (e *ExplainView) Stop() {}

func (e *ExplainView) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "j/k", Description: "Navigate"},
		{Key: "Enter", Description: "Expand/Collapse"},
		{Key: "r", Description: "Re-run"},
		{Key: "/", Description: "Filter"},
		{Key: "q/Esc", Description: "Back"},
	}
}

func (e *ExplainView) loadPlan() {
	provider := e.app.Provider()
	if provider == nil {
		e.app.ShowError("Not connected")
		return
	}

	async.NewLoader[*engine.PlanResult]().
		WithTimeout(30 * time.Second).
		OnSuccess(func(plan *engine.PlanResult) {
			e.plan = plan
			if plan.Root != nil {
				e.buildTree(plan)
			} else {
				e.showRawFallback(plan.RawText)
			}
		}).
		OnError(func(err error) {
			e.app.ShowError(fmt.Sprintf("Explain failed: %v", err))
			e.showRawFallback(fmt.Sprintf("Error: %v", err))
		}).
		Run(func(ctx context.Context) (*engine.PlanResult, error) {
			return provider.ExplainPlan(ctx, e.sql)
		})
}

func (e *ExplainView) buildTree(plan *engine.PlanResult) {
	maxCost := plan.MaxCost()

	root := e.convertNode(plan.Root, maxCost)
	e.tree.SetRoot(root)
	e.tree.ExpandAll()

	// Select root and show its details
	if plan.Root != nil {
		e.updateDetail(plan.Root)
	}
}

func (e *ExplainView) convertNode(pn *engine.PlanNode, maxCost float64) *components.TreeNode {
	label := pn.NodeType
	if pn.Relation != "" {
		label += " on " + pn.Relation
	}

	var details []string
	if pn.TotalCost > 0 {
		details = append(details, fmt.Sprintf("cost=%.1f", pn.TotalCost))
	}
	if pn.PlanRows > 0 {
		details = append(details, fmt.Sprintf("rows=%d", pn.PlanRows))
	}
	if pn.ActualRows > 0 {
		details = append(details, fmt.Sprintf("actual=%d", pn.ActualRows))
	}
	if pn.ActualTime > 0 {
		details = append(details, fmt.Sprintf("time=%.2fms", pn.ActualTime))
	}

	if len(details) > 0 {
		label += " (" + strings.Join(details, " ") + ")"
	}

	icon := nodeIcon(pn)

	// Color by cost using theme colors
	var nodeColor tcell.Color
	if maxCost > 0 {
		ratio := pn.TotalCost / maxCost
		if ratio > 0.75 {
			nodeColor = theme.Error()
		} else if ratio > 0.25 {
			nodeColor = theme.Warning()
		} else {
			nodeColor = theme.Success()
		}
	}

	// Bad estimate warning
	if pn.ActualRows > 0 && pn.PlanRows > 0 {
		ratio := float64(pn.ActualRows) / float64(pn.PlanRows)
		if ratio > 10 || ratio < 0.1 {
			icon = "⚠"
		}
	}

	node := &components.TreeNode{
		ID:       fmt.Sprintf("%p", pn),
		Label:    label,
		Icon:     icon,
		Color:    nodeColor,
		Data:     pn,
		Expanded: true,
	}

	for _, child := range pn.Children {
		node.AddChild(e.convertNode(child, maxCost))
	}

	return node
}

func nodeIcon(pn *engine.PlanNode) string {
	lower := strings.ToLower(pn.NodeType)
	switch {
	case strings.Contains(lower, "seq scan") || strings.Contains(lower, "scan"):
		return "󰍉"
	case strings.Contains(lower, "index"):
		return "󰆼"
	case strings.Contains(lower, "join"):
		return "󰌹"
	case strings.Contains(lower, "sort"):
		return "󰒺"
	case strings.Contains(lower, "aggregate") || strings.Contains(lower, "group"):
		return "󰃀"
	case strings.Contains(lower, "hash"):
		return "#"
	case strings.Contains(lower, "limit"):
		return "󰁅"
	default:
		return "•"
	}
}

func (e *ExplainView) updateDetail(pn *engine.PlanNode) {
	var b strings.Builder

	fmt.Fprintf(&b, "[::b]%s[::-]\n", pn.NodeType)
	if pn.Relation != "" {
		fmt.Fprintf(&b, "[%s]Table:[-] %s\n", theme.TagAccent(), pn.Relation)
	}
	if pn.IndexName != "" {
		fmt.Fprintf(&b, "[%s]Index:[-] %s\n", theme.TagAccent(), pn.IndexName)
	}
	if pn.Filter != "" {
		fmt.Fprintf(&b, "[%s]Filter:[-] %s\n", theme.TagAccent(), pn.Filter)
	}

	b.WriteString("\n[::b]Costs[::-]\n")
	fmt.Fprintf(&b, "  Total Cost: %.2f\n", pn.TotalCost)
	fmt.Fprintf(&b, "  Plan Rows:  %d\n", pn.PlanRows)
	if pn.ActualRows > 0 {
		fmt.Fprintf(&b, "  Actual Rows: %d\n", pn.ActualRows)
	}
	if pn.ActualTime > 0 {
		fmt.Fprintf(&b, "  Actual Time: %.2f ms\n", pn.ActualTime)
	}

	if pn.ActualRows > 0 && pn.PlanRows > 0 {
		ratio := float64(pn.ActualRows) / float64(pn.PlanRows)
		if ratio > 10 {
			fmt.Fprintf(&b, "\n[red]⚠ Bad estimate: actual/planned = %.1fx[-]\n", ratio)
		} else if ratio < 0.1 {
			fmt.Fprintf(&b, "\n[yellow]⚠ Overestimate: actual/planned = %.2fx[-]\n", ratio)
		}
	}

	if len(pn.Extra) > 0 {
		b.WriteString("\n[::b]Details[::-]\n")
		for k, v := range pn.Extra {
			fmt.Fprintf(&b, "  %s: %s\n", k, v)
		}
	}

	e.detailView.SetText(b.String())
	e.detailView.ScrollTo(0, 0)
}

func (e *ExplainView) showRawFallback(text string) {
	codeView := components.NewCodeView().
		SetCode(text).
		SetLanguage(components.LangSQL).
		SetShowLineNumbers(true)

	e.Split.SetTop(codeView)
	e.detailView.SetText("[dim]Raw EXPLAIN output (parsing not supported for this engine)[-]")
}

func (e *ExplainView) reload() {
	e.app.ShowInfo("Re-running EXPLAIN...")
	e.loadPlan()
}

func (e *ExplainView) showFilter() {
	e.app.ShowSearchMode("", components.SearchCallbacks{
		OnChange: func(text string) {
			e.tree.Filter(text)
		},
		OnSubmit: func(text string) {
			e.tree.Filter(text)
		},
		OnCancel: func() {
			e.tree.Filter("")
		},
	})
}

var _ nav.Component = (*ExplainView)(nil)
