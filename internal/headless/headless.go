package headless

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
)

// Options holds headless mode configuration.
type Options struct {
	Exec   string // SQL to execute directly
	Script string // Path to SQL file
	Format string // Output format: csv, json, tsv, table, sql
	Quiet  bool   // Suppress non-data output
}

// Exit codes
const (
	ExitSuccess    = 0
	ExitQueryError = 1
	ExitConnError  = 2
)

// Run executes queries in headless mode without the TUI.
func Run(cfg *config.Config, connCfg config.ConnectionConfig, opts Options) int {
	if opts.Format == "" {
		opts.Format = "table"
	}

	// Resolve SQL from flags, file, or stdin
	sql, err := resolveSQL(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitQueryError
	}

	if sql == "" {
		fmt.Fprintf(os.Stderr, "Error: no SQL provided\n")
		return ExitQueryError
	}

	// Create and connect provider
	connCfg = connCfg.ExpandEnv()
	provider, err := engine.NewProvider(connCfg.Engine)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitConnError
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := provider.Connect(ctx, connCfg); err != nil {
		cancel()
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		return ExitConnError
	}
	cancel()
	defer provider.Close()

	if !opts.Quiet {
		fmt.Fprintf(os.Stderr, "Connected to %s\n", connCfg.Engine)
	}

	// Split on semicolons for multi-statement scripts
	statements := splitStatements(sql)

	var lastResult *engine.QueryResult
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		result, err := provider.ExecuteQuery(ctx, stmt)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Query error: %v\n", err)
			return ExitQueryError
		}

		// For non-SELECT statements, print message to stderr
		if result.Message != "" && !opts.Quiet {
			fmt.Fprintf(os.Stderr, "%s\n", result.Message)
		}

		if len(result.Columns) > 0 {
			lastResult = result
		}
	}

	// Output the last SELECT result to stdout
	if lastResult != nil {
		output := Format(lastResult, opts.Format)
		fmt.Print(output)
	}

	return ExitSuccess
}

func resolveSQL(opts Options) (string, error) {
	if opts.Exec != "" {
		return opts.Exec, nil
	}

	if opts.Script != "" {
		data, err := os.ReadFile(opts.Script)
		if err != nil {
			return "", fmt.Errorf("reading script file: %w", err)
		}
		return string(data), nil
	}

	// Check if stdin has data (piped)
	info, err := os.Stdin.Stat()
	if err != nil {
		return "", nil
	}
	if (info.Mode() & os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}

	return "", nil
}

func splitStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inSingleQuote := false
	inDoubleQuote := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		if ch == '\'' && !inDoubleQuote {
			inSingleQuote = !inSingleQuote
		} else if ch == '"' && !inSingleQuote {
			inDoubleQuote = !inDoubleQuote
		}

		if ch == ';' && !inSingleQuote && !inDoubleQuote {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteByte(ch)
	}

	// Don't forget trailing statement without semicolon
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}
