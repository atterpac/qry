package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/jig/theme"
	"github.com/rivo/tview"
)

// ConnectionInfo shows server version, capabilities, and connection details.
type ConnectionInfo struct {
	*tview.TextView
	app *App
}

func NewConnectionInfo(app *App) *ConnectionInfo {
	c := &ConnectionInfo{
		TextView: tview.NewTextView(),
		app:      app,
	}
	c.SetDynamicColors(true)
	c.SetWordWrap(true)
	c.SetBorder(true)
	c.SetTitle(" Connection Info ")
	c.SetTitleAlign(tview.AlignLeft)
	theme.Register(c.TextView)
	return c
}

func (c *ConnectionInfo) Name() string { return "Connection Info" }

func (c *ConnectionInfo) Start() {
	c.loadInfo()
}

func (c *ConnectionInfo) Stop() {}

func (c *ConnectionInfo) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "r", Description: "Refresh"},
	}
}

func (c *ConnectionInfo) loadInfo() {
	provider := c.app.Provider()
	if provider == nil {
		c.SetText("[red]Not connected[-]")
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		version, vErr := provider.ServerVersion(ctx)
		pErr := provider.Ping(ctx)
		caps := provider.Capabilities()

		c.app.QueueUpdateDraw(func() {
			var b strings.Builder

			fmt.Fprintf(&b, "[::b]Engine[::-]       %s\n", provider.EngineType())
			fmt.Fprintf(&b, "[::b]Profile[::-]      %s\n", c.app.ActiveProfileName())

			if vErr == nil {
				fmt.Fprintf(&b, "[::b]Version[::-]      %s\n", version)
			} else {
				fmt.Fprintf(&b, "[::b]Version[::-]      [red]error: %v[-]\n", vErr)
			}

			if pErr == nil {
				fmt.Fprintf(&b, "[::b]Status[::-]       [green]Connected[-]\n")
			} else {
				fmt.Fprintf(&b, "[::b]Status[::-]       [red]Disconnected: %v[-]\n", pErr)
			}

			// Connection details
			profileName, profile := c.app.Config().GetActiveProfile()
			_ = profileName

			if profile.DSN != "" {
				fmt.Fprintf(&b, "[::b]DSN[::-]          %s\n", redactDSN(profile.DSN))
			}
			if profile.Path != "" {
				fmt.Fprintf(&b, "[::b]Path[::-]         %s\n", profile.Path)
			}
			if profile.URL != "" {
				fmt.Fprintf(&b, "[::b]URL[::-]          %s\n", profile.URL)
			}
			if profile.Database != "" {
				fmt.Fprintf(&b, "[::b]Database[::-]     %s\n", profile.Database)
			}
			if profile.Namespace != "" {
				fmt.Fprintf(&b, "[::b]Namespace[::-]    %s\n", profile.Namespace)
			}

			fmt.Fprintf(&b, "\n[::b]Capabilities[::-]\n")
			dim := theme.TagFgDim()
			fmt.Fprintf(&b, "  Schemas:      %s\n", boolTag(caps.HasSchemas))
			fmt.Fprintf(&b, "  Databases:    %s\n", boolTag(caps.HasDatabases))
			fmt.Fprintf(&b, "  Namespaces:   %s\n", boolTag(caps.HasNamespaces))
			fmt.Fprintf(&b, "  RETURNING:    %s\n", boolTag(caps.SupportsReturning))
			fmt.Fprintf(&b, "  Record Links: %s\n", boolTag(caps.HasRecordLinks))
			fmt.Fprintf(&b, "  Graph Queries:%s\n", boolTag(caps.HasGraphQueries))
			fmt.Fprintf(&b, "  Identifier:   [%s]%s[-]\n", dim, caps.IdentifierQuote)

			c.SetText(b.String())
		})
	}()
}

func boolTag(v bool) string {
	if v {
		return "[green]yes[-]"
	}
	return "[red]no[-]"
}

var _ nav.Component = (*ConnectionInfo)(nil)
