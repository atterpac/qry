package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/qry/internal/command"
	"github.com/atterpac/qry/internal/engine"
)

// buildCommandTable constructs the command-bar registry. Each command's Run
// closure captures the app, so the dispatch/alias logic lives in the command
// package while the bodies retain access to view-layer internals.
func (a *App) buildCommandTable() *command.Table {
	t := command.NewTable()

	t.Add(&command.Command{
		Name: "tables", Aliases: []string{"schema"}, Usage: "tables",
		Run: func(args []string) { a.NavigateToSchemaExplorer() },
	})
	t.Add(&command.Command{
		Name: "editor", Aliases: []string{"e"}, Usage: "editor [sql]",
		Run: func(args []string) {
			if len(args) > 0 {
				a.NavigateToQueryEditorWithSQL(strings.Join(args, " "))
			} else {
				a.NavigateToQueryEditor()
			}
		},
	})
	t.Add(&command.Command{
		Name: "info", Usage: "info",
		Run: func(args []string) { a.NavigateToConnectionInfo() },
	})
	t.Add(&command.Command{
		Name: "databases", Aliases: []string{"db"}, Usage: "databases",
		Run: func(args []string) { a.NavigateToDatabaseList() },
	})
	t.Add(&command.Command{
		Name: "queries", Usage: "queries",
		Run: func(args []string) { a.NavigateToQueryList() },
	})
	t.Add(&command.Command{
		Name: "history", Usage: "history",
		Run: func(args []string) { a.NavigateToQueryHistory() },
	})
	t.Add(&command.Command{
		Name: "run", Usage: "run <sql>",
		Run: func(args []string) {
			if len(args) > 0 {
				a.NavigateToQueryEditorWithSQL(strings.Join(args, " "))
			}
		},
	})
	t.Add(&command.Command{
		Name: "table", Usage: "table <name>",
		Run: func(args []string) {
			if len(args) > 0 {
				a.NavigateToTableData("", args[0])
			}
		},
	})
	t.Add(&command.Command{
		Name: "erd", Usage: "erd [schema]",
		Run: func(args []string) {
			schema := "public"
			if len(args) > 0 {
				schema = args[0]
			}
			a.NavigateToERD(schema)
		},
	})
	t.Add(&command.Command{
		Name: "profile", Usage: "profile",
		Run: func(args []string) { a.showProfileSelector() },
	})
	t.Add(&command.Command{
		Name: "quit", Aliases: []string{"q"}, Usage: "quit",
		Run: func(args []string) { a.app.Stop() },
	})
	t.Add(&command.Command{
		Name: "sort", Usage: "sort <column> [desc]",
		Run: a.cmdSort,
	})
	t.Add(&command.Command{
		Name: "count", Usage: "count",
		Run: func(args []string) {
			if td, ok := a.requireTableData(); ok {
				td.runCount()
			}
		},
	})
	t.Add(&command.Command{
		Name: "describe", Aliases: []string{"desc"}, Usage: "describe",
		Run: func(args []string) {
			if td, ok := a.requireTableData(); ok {
				td.showSchemaOverlay()
			}
		},
	})
	t.Add(&command.Command{
		Name: "begin", Usage: "begin",
		Run: a.cmdBegin,
	})
	t.Add(&command.Command{
		Name: "commit", Usage: "commit",
		Run: a.cmdCommit,
	})
	t.Add(&command.Command{
		Name: "rollback", Usage: "rollback",
		Run: a.cmdRollback,
	})
	t.Add(&command.Command{
		Name: "discard", Usage: "discard",
		Run: func(args []string) {
			if td, ok := a.requireTableData(); ok {
				td.discardChanges()
			}
		},
	})
	t.Add(&command.Command{
		Name: "dry-run", Usage: "dry-run",
		Run: func(args []string) {
			a.ShowInfo("Dry-run: wrapping pending changes in BEGIN/ROLLBACK")
			if td, ok := a.requireTableData(); ok {
				td.dryRun()
			}
		},
	})
	t.Add(&command.Command{
		Name: "diff", Usage: "diff schema <profile> [schema]",
		Run: a.cmdDiff,
	})
	t.Add(&command.Command{
		Name: "watch", Usage: "watch <duration>|stop",
		Run: a.cmdWatch,
	})
	t.Add(&command.Command{
		Name: "explain", Usage: "explain <sql>",
		Run: a.cmdExplain,
	})
	t.Add(&command.Command{
		Name: "pipe", Usage: "pipe [--format csv|json|tsv] <shell command>",
		Run: a.cmdPipe,
	})

	return t
}

// requireTableData returns the current TableData view, warning the user when
// the active view is not a table.
func (a *App) requireTableData() (*TableData, bool) {
	td, ok := a.currentTableData()
	if !ok {
		a.ShowWarning("Not viewing a table")
	}
	return td, ok
}

func (a *App) cmdSort(args []string) {
	td, ok := a.requireTableData()
	if !ok {
		return
	}
	if len(args) == 0 {
		a.ShowWarning("Usage: sort <column> [desc]")
		return
	}
	col := args[0]
	// Validate column exists
	found := false
	for _, rc := range td.resultCols {
		if strings.EqualFold(rc, col) {
			col = rc // use exact case
			found = true
			break
		}
	}
	if !found {
		a.ShowWarning(fmt.Sprintf("Column %q not found", col))
		return
	}
	dir := "ASC"
	if len(args) > 1 && strings.EqualFold(args[1], "desc") {
		dir = "DESC"
	}
	td.SetSort(col, dir)
}

func (a *App) cmdBegin(args []string) {
	tp, ok := a.Provider().(engine.TransactionalProvider)
	if !ok {
		a.ShowWarning("Current engine does not support transactions")
		return
	}
	if tp.InTransaction() {
		a.ShowWarning("Transaction already active")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.BeginTx(ctx); err != nil {
		a.ShowError(fmt.Sprintf("Begin failed: %v", err))
		return
	}
	a.txMode = true
	a.updateTxStatus()
	a.ShowSuccess("Transaction started")
}

func (a *App) cmdCommit(args []string) {
	tp, ok := a.Provider().(engine.TransactionalProvider)
	if !ok || !tp.InTransaction() {
		a.ShowWarning("No active transaction")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tp.CommitTx(ctx); err != nil {
		a.ShowError(fmt.Sprintf("Commit failed: %v", err))
		return
	}
	a.txMode = false
	a.updateTxStatus()
	a.ShowSuccess("Transaction committed")
	if td, ok := a.currentTableData(); ok {
		td.discardChanges()
	}
}

func (a *App) cmdRollback(args []string) {
	tp, ok := a.Provider().(engine.TransactionalProvider)
	if !ok || !tp.InTransaction() {
		a.ShowWarning("No active transaction (use :discard to clear pending edits)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tp.RollbackTx(ctx); err != nil {
		a.ShowError(fmt.Sprintf("Rollback failed: %v", err))
		return
	}
	a.txMode = false
	a.updateTxStatus()
	a.ShowSuccess("Transaction rolled back")
	if td, ok := a.currentTableData(); ok {
		td.discardChanges()
	}
}

func (a *App) cmdDiff(args []string) {
	if len(args) < 2 || args[0] != "schema" {
		a.ShowWarning("Usage: diff schema <profile> [schema]")
		return
	}
	targetProfile := args[1]
	schema := ""
	if len(args) > 2 {
		schema = args[2]
	}
	a.NavigateToSchemaDiff(targetProfile, schema)
}

func (a *App) cmdWatch(args []string) {
	c := a.app.Pages().Current()
	qe, ok := c.(*QueryEditor)
	if !ok {
		a.ShowWarning("Watch mode is only available in the query editor")
		return
	}
	if len(args) > 0 && args[0] == "stop" {
		qe.stopWatch()
		return
	}
	if len(args) == 0 {
		a.ShowWarning("Usage: watch <duration> (e.g. 5s, 1m) or watch stop")
		return
	}
	dur, err := time.ParseDuration(args[0])
	if err != nil {
		a.ShowWarning(fmt.Sprintf("Invalid duration: %v", err))
		return
	}
	qe.startWatch(dur)
}

func (a *App) cmdExplain(args []string) {
	if len(args) == 0 {
		// Try to get SQL from current query editor
		if qe, ok := a.app.Pages().Current().(*QueryEditor); ok && qe.lastSQL != "" {
			a.NavigateToExplainView(qe.lastSQL)
			return
		}
		a.ShowWarning("Usage: explain <sql>")
		return
	}
	a.NavigateToExplainView(strings.Join(args, " "))
}

func (a *App) cmdPipe(args []string) {
	if len(args) == 0 {
		a.ShowWarning("Usage: pipe [--format csv|json|tsv] <shell command>")
		return
	}
	format := "json"
	shellArgs := args
	if len(shellArgs) >= 2 && shellArgs[0] == "--format" {
		format = shellArgs[1]
		shellArgs = shellArgs[2:]
	}
	if len(shellArgs) == 0 {
		a.ShowWarning("Usage: pipe [--format csv|json|tsv] <shell command>")
		return
	}
	pv := a.currentPipeableView()
	if pv == nil {
		a.ShowWarning("Current view has no data to pipe")
		return
	}
	data := pv.BuildPipeData(format)
	if data == "" {
		a.ShowWarning("No data to pipe")
		return
	}
	a.executePipe(data, strings.Join(shellArgs, " "))
}
