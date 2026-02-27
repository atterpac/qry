package view

import (
	"github.com/atterpac/jig/components"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ConfirmAction shows a confirmation modal with a message.
// onConfirm is called if the user presses Enter or 'y'.
func ConfirmAction(app *App, title, message string, onConfirm func()) {
	tv := tview.NewTextView()
	tv.SetDynamicColors(true)
	tv.SetText(message + "\n\n[dim]Press Enter to confirm, Esc to cancel[-]")
	tv.SetTextAlign(tview.AlignCenter)

	tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter || event.Rune() == 'y' {
			app.app.Pages().Pop()
			onConfirm()
			return nil
		}
		return event
	})

	modal := components.NewModal(components.ModalConfig{
		Title:  title,
		Width:  50,
		Height: 8,
	}).SetContent(tv)

	app.app.Pages().Push(modal)
}
