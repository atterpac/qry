package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/atterpac/dado/theme"
	"github.com/atterpac/dado/theme/themes"
	"github.com/gdamore/tcell/v2"

	"github.com/atterpac/qry/internal/config"
	"github.com/atterpac/qry/internal/engine"
	"github.com/atterpac/qry/internal/headless"
	"github.com/atterpac/qry/internal/view"
)

const splashLogo = ` ░▒▓██████▓▒░░▒▓███████▓▒░░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓███████▓▒░ ░▒▓██████▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░  ░▒▓█▓▒░
░▒▓█▓▒░░▒▓█▓▒░▒▓█▓▒░░▒▓█▓▒░  ░▒▓█▓▒░
 ░▒▓██████▓▒░░▒▓█▓▒░░▒▓█▓▒░  ░▒▓█▓▒░
   ░▒▓█▓▒░
    ░▒▓██▓▒░                             `

var version = "dev"

func main() {
	var (
		flagProfile string
		flagDSN     string
		flagPath    string
		flagEngine  string
		flagTheme   string
		flagVersion bool
		flagExec    string
		flagScript  string
		flagFormat  string
		flagQuiet   bool
	)

	flag.StringVar(&flagProfile, "profile", "", "connection profile name")
	flag.StringVar(&flagDSN, "dsn", "", "database DSN (overrides profile)")
	flag.StringVar(&flagPath, "path", "", "SQLite file path (overrides profile)")
	flag.StringVar(&flagEngine, "engine", "", "database engine: postgres, sqlite, mysql, surrealdb")
	flag.StringVar(&flagTheme, "theme", "", "color theme (overrides config)")
	flag.BoolVar(&flagVersion, "version", false, "print version and exit")
	flag.StringVar(&flagExec, "exec", "", "execute SQL and exit (headless mode)")
	flag.StringVar(&flagScript, "script", "", "execute SQL file and exit (headless mode)")
	flag.StringVar(&flagFormat, "format", "table", "output format: csv, json, tsv, table, sql (headless mode)")
	flag.BoolVar(&flagQuiet, "quiet", false, "suppress non-data output (headless mode)")
	flag.Parse()

	if flagVersion {
		fmt.Printf("qry %s\n", version)
		os.Exit(0)
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Determine theme
	themeName := cfg.Theme
	if flagTheme != "" {
		themeName = flagTheme
	}
	if themeName == "" {
		themeName = themes.DefaultName
	}

	selectedTheme := themes.Get(themeName)
	if selectedTheme == nil {
		selectedTheme = themes.Default()
	}
	theme.SetProvider(selectedTheme)

	// Determine active profile
	activeProfile := cfg.ActiveProfile
	if flagProfile != "" {
		if err := cfg.SetActiveProfile(flagProfile); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		activeProfile = flagProfile
	}

	_, connCfg := cfg.GetActiveProfile()
	connCfg = connCfg.ExpandEnv()

	// CLI overrides
	if flagDSN != "" {
		connCfg.DSN = flagDSN
	}
	if flagPath != "" {
		connCfg.Path = flagPath
		if connCfg.Engine == "" {
			connCfg.Engine = config.EngineSQLite
		}
	}
	if flagEngine != "" {
		connCfg.Engine = config.EngineType(flagEngine)
	}

	// Determine if CLI flags override the connection
	hasCLIOverride := flagDSN != "" || flagPath != "" || flagEngine != "" || flagProfile != ""

	// Detect headless mode: --exec, --script, or piped stdin
	isHeadless := flagExec != "" || flagScript != ""
	if !isHeadless {
		if info, err := os.Stdin.Stat(); err == nil {
			isHeadless = (info.Mode() & os.ModeCharDevice) == 0
		}
	}

	if isHeadless {
		code := headless.Run(cfg, connCfg, headless.Options{
			Exec:   flagExec,
			Script: flagScript,
			Format: flagFormat,
			Quiet:  flagQuiet,
		})
		os.Exit(code)
	}

	var provider engine.Provider
	if !cfg.HasUserProfiles() && !hasCLIOverride {
		// No user-configured profiles and no CLI overrides — let the app
		// show the first-run setup modal instead of auto-connecting.
		provider = nil
	} else {
		// Connect with splash screen
		var err error
		provider, err = connectWithUI(connCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer provider.Close()
	}

	// Launch main app
	app := view.NewApp(provider, cfg, activeProfile)
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func connectWithUI(cfg config.ConnectionConfig) (engine.Provider, error) {
	app := core.NewApp()

	splash := components.NewSplash().
		SetLogo(splashLogo).
		SetStatusHeight(4).
		SetGradient(theme.GradientDiagonal).
		SetDismissKeys(nil)

	splash.Build()

	updateStatus := func(connectionStatus string) {
		tagline := fmt.Sprintf("[%s]made with ♥  by atterpac[-]", theme.TagFgDim())
		splash.SetStatus(tagline + "\n" + connectionStatus)
	}
	updateStatus(fmt.Sprintf("[%s]Initializing...[-]", theme.TagFgDim()))

	type result struct {
		provider engine.Provider
		err      error
	}
	done := make(chan result, 1)
	quit := make(chan struct{})

	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'q' || event.Key() == tcell.KeyCtrlC {
			close(quit)
			app.Stop()
			return nil
		}
		return event
	})

	go func() {
		time.Sleep(500 * time.Millisecond)

		const maxRetries = 5
		backoff := time.Second

		for attempt := 1; attempt <= maxRetries; attempt++ {
			select {
			case <-quit:
				done <- result{err: fmt.Errorf("cancelled")}
				return
			default:
			}

			engineLabel := string(cfg.Engine)
			connTarget := cfg.DSN
			if connTarget == "" {
				connTarget = cfg.Path
			}
			if connTarget == "" {
				connTarget = cfg.URL
			}

			app.QueueUpdateDraw(func() {
				updateStatus(fmt.Sprintf("[yellow]Connecting to %s (%s)... (attempt %d/%d)[-]", connTarget, engineLabel, attempt, maxRetries))
			})

			provider, err := engine.NewProvider(cfg.Engine)
			if err != nil {
				done <- result{err: err}
				app.Stop()
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			connectErr := provider.Connect(ctx, cfg)
			cancel()

			if connectErr == nil {
				app.QueueUpdateDraw(func() {
					updateStatus("[green]Connected![-]")
				})
				time.Sleep(500 * time.Millisecond)
				done <- result{provider: provider}
				app.Stop()
				return
			}

			if attempt < maxRetries {
				app.QueueUpdateDraw(func() {
					updateStatus(fmt.Sprintf("[red]Failed: %v[-]\n[dim]Retrying in %s...[-]", connectErr, backoff))
				})
				select {
				case <-quit:
					done <- result{err: fmt.Errorf("cancelled")}
					return
				case <-time.After(backoff):
				}
				backoff = min(backoff*2, 10*time.Second)
			} else {
				app.QueueUpdateDraw(func() {
					updateStatus(fmt.Sprintf("[red]Failed after %d attempts: %v[-]\n[dim]Press 'q' to quit[-]", maxRetries, connectErr))
				})
				<-quit
				done <- result{err: connectErr}
				return
			}
		}
	}()

	app.SetRoot(splash)
	if err := app.Run(); err != nil {
		return nil, err
	}

	res := <-done
	return res.provider, res.err
}
