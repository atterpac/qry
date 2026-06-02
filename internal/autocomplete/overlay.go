package autocomplete

import (
	"github.com/atterpac/dado/theme"
	"github.com/gdamore/tcell/v2"
)

const defaultMaxVisible = 8

// Overlay is a floating dropdown that draws autocomplete suggestions directly
// onto a tcell.Screen. It is not a tview Primitive — the owning component
// is responsible for calling Draw and routing input.
type Overlay struct {
	visible       bool
	suggestions   []Suggestion
	selectedIndex int
	maxVisible    int
	scrollOffset  int
	anchorX       int // screen column of the dropdown's left edge
	anchorY       int // screen row of the dropdown's top edge

	OnAccept  func(Suggestion)
	OnDismiss func()
}

// NewOverlay creates an overlay with default settings.
func NewOverlay() *Overlay {
	return &Overlay{
		maxVisible: defaultMaxVisible,
	}
}

// Visible reports whether the overlay is currently shown.
func (o *Overlay) Visible() bool { return o.visible }

// Suggestions returns the current suggestion list.
func (o *Overlay) Suggestions() []Suggestion { return o.suggestions }

// Show displays the overlay with the given suggestions at the given screen position.
func (o *Overlay) Show(suggestions []Suggestion, x, y int) {
	if len(suggestions) == 0 {
		o.Hide()
		return
	}
	o.suggestions = suggestions
	o.selectedIndex = 0
	o.scrollOffset = 0
	o.anchorX = x
	o.anchorY = y
	o.visible = true
}

// Hide dismisses the overlay.
func (o *Overlay) Hide() {
	o.visible = false
	o.suggestions = nil
	o.selectedIndex = 0
	o.scrollOffset = 0
}

// HandleKey processes a key event. Returns true if the event was consumed.
func (o *Overlay) HandleKey(event *tcell.EventKey) bool {
	if !o.visible {
		return false
	}

	switch event.Key() {
	case tcell.KeyUp:
		if o.selectedIndex > 0 {
			o.selectedIndex--
			if o.selectedIndex < o.scrollOffset {
				o.scrollOffset = o.selectedIndex
			}
		}
		return true

	case tcell.KeyDown:
		if o.selectedIndex < len(o.suggestions)-1 {
			o.selectedIndex++
			if o.selectedIndex >= o.scrollOffset+o.maxVisible {
				o.scrollOffset = o.selectedIndex - o.maxVisible + 1
			}
		}
		return true

	case tcell.KeyTab, tcell.KeyEnter:
		if o.selectedIndex < len(o.suggestions) && o.OnAccept != nil {
			o.OnAccept(o.suggestions[o.selectedIndex])
		}
		o.Hide()
		return true

	case tcell.KeyEscape:
		o.Hide()
		if o.OnDismiss != nil {
			o.OnDismiss()
		}
		return true
	}

	return false
}

// Draw renders the overlay onto the screen. Call this after the parent's Draw.
func (o *Overlay) Draw(screen tcell.Screen) {
	if !o.visible || len(o.suggestions) == 0 {
		return
	}

	screenW, screenH := screen.Size()

	// Compute dimensions
	visibleCount := len(o.suggestions)
	if visibleCount > o.maxVisible {
		visibleCount = o.maxVisible
	}

	// Calculate widths
	maxTextW := 0
	maxCatW := 0
	maxDescW := 0
	for _, s := range o.suggestions {
		if len(s.Text) > maxTextW {
			maxTextW = len(s.Text)
		}
		if len(s.Category) > maxCatW {
			maxCatW = len(s.Category)
		}
		if len(s.Description) > maxDescW {
			maxDescW = len(s.Description)
		}
	}

	// Content width: [category] text  description
	contentW := maxTextW + 2 // text + padding
	if maxCatW > 0 {
		contentW += maxCatW + 3 // [cat] + space
	}
	if maxDescW > 0 {
		contentW += maxDescW + 2 // space + desc
	}

	// Box dimensions (border adds 2 to each dimension)
	boxW := contentW + 2
	boxH := visibleCount + 2

	// Cap width
	if boxW > screenW-2 {
		boxW = screenW - 2
		contentW = boxW - 2
	}

	// Position: prefer below cursor, flip above if no room
	x := o.anchorX
	y := o.anchorY + 1 // one row below cursor

	if x+boxW > screenW {
		x = screenW - boxW
	}
	if x < 0 {
		x = 0
	}

	if y+boxH > screenH {
		// Flip above
		y = o.anchorY - boxH
		if y < 0 {
			y = 0
		}
	}

	// Colors
	bgColor := theme.BgLight()
	fgColor := theme.Fg()
	borderColor := theme.Border()
	selBg := theme.Accent()
	selFg := tcell.ColorBlack
	catColor := theme.FgDim()
	descColor := theme.FgMuted()

	normalStyle := tcell.StyleDefault.Background(bgColor).Foreground(fgColor)
	borderStyle := tcell.StyleDefault.Background(bgColor).Foreground(borderColor)
	selectedStyle := tcell.StyleDefault.Background(selBg).Foreground(selFg).Bold(true)
	catStyle := tcell.StyleDefault.Background(bgColor).Foreground(catColor)
	descStyle := tcell.StyleDefault.Background(bgColor).Foreground(descColor)
	selCatStyle := tcell.StyleDefault.Background(selBg).Foreground(selFg)
	selDescStyle := tcell.StyleDefault.Background(selBg).Foreground(selFg)

	// Draw border
	DrawBox(screen, x, y, boxW, boxH, borderStyle)

	// Draw items
	for vi := 0; vi < visibleCount; vi++ {
		idx := o.scrollOffset + vi
		if idx >= len(o.suggestions) {
			break
		}

		s := o.suggestions[idx]
		isSelected := idx == o.selectedIndex
		row := y + 1 + vi

		// Clear row
		style := normalStyle
		cs := catStyle
		ds := descStyle
		if isSelected {
			style = selectedStyle
			cs = selCatStyle
			ds = selDescStyle
		}

		// Fill row background
		for col := x + 1; col < x+boxW-1; col++ {
			screen.SetContent(col, row, ' ', nil, style)
		}

		col := x + 1

		// Category tag
		if maxCatW > 0 && s.Category != "" {
			col = DrawString(screen, col, row, "[", cs)
			col = DrawString(screen, col, row, s.Category, cs)
			col = DrawString(screen, col, row, "]", cs)
			col = DrawString(screen, col, row, " ", style)
		}

		// Text
		col = DrawString(screen, col, row, s.Text, style)

		// Description (right-aligned-ish)
		if s.Description != "" && col+2+len(s.Description) < x+boxW-1 {
			col = DrawString(screen, col, row, "  ", style)
			DrawString(screen, col, row, s.Description, ds)
		}
	}

	// Scrollbar indicator
	if len(o.suggestions) > o.maxVisible {
		// Simple scroll indicator on right border
		scrollRange := boxH - 2
		thumbPos := 0
		if len(o.suggestions) > 1 {
			thumbPos = o.scrollOffset * (scrollRange - 1) / (len(o.suggestions) - o.maxVisible)
		}
		screen.SetContent(x+boxW-1, y+1+thumbPos, '█', nil, borderStyle)
	}
}

func DrawBox(screen tcell.Screen, x, y, w, h int, style tcell.Style) {
	// Corners
	screen.SetContent(x, y, '┌', nil, style)
	screen.SetContent(x+w-1, y, '┐', nil, style)
	screen.SetContent(x, y+h-1, '└', nil, style)
	screen.SetContent(x+w-1, y+h-1, '┘', nil, style)

	// Horizontal borders
	for col := x + 1; col < x+w-1; col++ {
		screen.SetContent(col, y, '─', nil, style)
		screen.SetContent(col, y+h-1, '─', nil, style)
	}

	// Vertical borders
	for row := y + 1; row < y+h-1; row++ {
		screen.SetContent(x, row, '│', nil, style)
		screen.SetContent(x+w-1, row, '│', nil, style)
	}
}

func DrawString(screen tcell.Screen, x, y int, s string, style tcell.Style) int {
	for _, ch := range s {
		screen.SetContent(x, y, ch, nil, style)
		x++
	}
	return x
}
