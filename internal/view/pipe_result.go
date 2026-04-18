package view

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/atterpac/jig/components"
	"github.com/atterpac/jig/nav"
	"github.com/atterpac/qry/internal/clipboard"
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

	p.CodeView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Rune() == 'y' {
			if err := clipboard.Copy(output); err != nil {
				app.ShowError(fmt.Sprintf("Copy failed: %v", err))
			} else {
				app.ShowSuccess("Output copied to clipboard")
			}
			return nil
		}
		return event
	})

	return p
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
	go func() {
		cmd := exec.Command("sh", "-c", shellCmd)
		cmd.Stdin = strings.NewReader(data)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err := cmd.Run()

		a.QueueUpdateDraw(func() {
			if err != nil {
				errMsg := stderr.String()
				if errMsg == "" {
					errMsg = err.Error()
				}
				a.ShowError(fmt.Sprintf("Pipe failed: %s", errMsg))
				return
			}

			output := stdout.String()
			if output == "" {
				a.ShowInfo("Pipe produced no output")
				return
			}

			view := NewPipeResultView(a, output, shellCmd)
			a.app.Pages().Push(view)
		})
	}()
}
