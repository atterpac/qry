package view

import (
	"github.com/atterpac/dado/components"
	"github.com/atterpac/dado/core"
	"github.com/gdamore/tcell/v2"
)

// confirmContent wraps a TextView and also handles the 'y' key as confirmation.
type confirmContent struct {
	*core.TextView
	onConfirm func()
}

func (c *confirmContent) HandleKey(ev *tcell.EventKey) bool {
	if ev.Key() == tcell.KeyEnter || ev.Rune() == 'y' {
		c.onConfirm()
		return true
	}
	return c.TextView.HandleKey(ev)
}

// ConfirmAction shows a confirmation modal with a message.
// onConfirm is called if the user presses Enter or 'y'.
func ConfirmAction(app *App, title, message string, onConfirm func()) {
	tv := core.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetText(message + "\n\n[dim]Press Enter to confirm, Esc to cancel[-]")
	tv.SetTextAlign(core.AlignCenter)

	content := &confirmContent{TextView: tv, onConfirm: func() {
		app.app.Pages().Pop()
		onConfirm()
	}}

	modal := components.NewModal(components.ModalConfig{
		Title:  title,
		Width:  50,
		Height: 8,
	}).SetContent(content).
		SetOnSubmit(func() {
			app.app.Pages().Pop()
			onConfirm()
		}).
		SetOnCancel(func() {
			app.app.Pages().Pop()
		})

	app.app.Pages().Push(modal)
}
