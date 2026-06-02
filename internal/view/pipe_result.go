package view

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/atterpac/dado/async"
	"github.com/atterpac/dado/clipboard"
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/nav"
	"github.com/gdamore/tcell/v2"
)

// PipeResultView is a full-screen view showing shell pipe output.
type PipeResultView struct {
	*components.CodeView
	app     *App
	output  string
	command string
}

func NewPipeResultView(app *App, output, command string) *PipeResultView {
	p := &PipeResultView{
		app:     app,
		output:  output,
		command: command,
	}

	p.CodeView = components.NewCodeView().
		SetCode(output).
		SetShowLineNumbers(true).
		SetWrapLines(true)

	// Auto-detect language from output
	trimmed := strings.TrimSpace(output)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		p.CodeView.SetLanguage(components.LangJSON)
	} else {
		p.CodeView.SetLanguage(components.LangNone)
	}

	return p
}

// HandleKey intercepts `y` to copy the output, delegating all other keys
// (scrolling, etc.) to the embedded CodeView.
func (p *PipeResultView) HandleKey(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyRune && ev.Rune() == 'y' {
		if err := clipboard.Copy(p.output); err != nil {
			p.app.ShowError(fmt.Sprintf("Copy failed: %v", err))
		} else {
			p.app.ShowSuccess("Output copied to clipboard")
		}
		return true
	}
	return p.CodeView.HandleKey(ev)
}

func (p *PipeResultView) Name() string {
	title := fmt.Sprintf("Pipe: %s", p.command)
	if len(title) > 60 {
		title = title[:57] + "..."
	}
	return title
}

func (p *PipeResultView) Start() {}
func (p *PipeResultView) Stop()  {}

func (p *PipeResultView) Hints() []components.KeyHint {
	return []components.KeyHint{
		{Key: "y", Description: "Copy output"},
		{Key: "j/k", Description: "Scroll"},
		{Key: "q/Esc", Description: "Back"},
	}
}

var _ nav.Component = (*PipeResultView)(nil)

func (a *App) executePipe(data, shellCmd string) {
	async.NewLoader[string]().
		OnSuccess(func(output string) {
			if output == "" {
				a.ShowInfo("Pipe produced no output")
				return
			}

			view := NewPipeResultView(a, output, shellCmd)
			a.app.Pages().Push(view)
		}).
		OnError(func(err error) {
			a.ShowError(fmt.Sprintf("Pipe failed: %s", err))
		}).
		Run(func(ctx context.Context) (string, error) {
			cmd := exec.CommandContext(ctx, "sh", "-c", shellCmd)
			cmd.Stdin = strings.NewReader(data)

			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				errMsg := stderr.String()
				if errMsg == "" {
					errMsg = err.Error()
				}
				return "", fmt.Errorf("%s", errMsg)
			}

			return stdout.String(), nil
		})
}
