package ui

import (
	"applauncher/internal/core"
	"applauncher/internal/win"
)

// Palette, in 0x00BBGGRR order (the byte order GDI expects).
const (
	ColorBackground uint32 = 0x1F1A1A // #1A1A1F
	colorSelection  uint32 = 0x332A2A // #2A2A33
	colorHover      uint32 = 0x2A2323 // #23232A
	colorSeparator  uint32 = 0x382F2F // #2F2F38
	colorText       uint32 = 0xF1ECEC // #ECECF1
	colorDim        uint32 = 0x968A8A // #8A8A96
	colorAccent     uint32 = 0xFFA27A // #7AA2FF
	colorError      uint32 = 0x6B6BFF // #FF6B6B
	colorBorder     uint32 = 0x443A3A // #3A3A44
)

type layout struct {
	pad      int32
	inputTop int32
	inputH   int32
	listTop  int32
	rowH     int32
	rows     int32
	footerY  int32
}

func (a *App) scale(v int32) int32 { return v * a.dpi / 96 }

func (a *App) computeLayout() layout {
	l := layout{
		pad:      a.scale(18),
		inputTop: a.scale(14),
		inputH:   a.scale(32),
		rowH:     a.scale(50),
		footerY:  a.height - a.scale(10),
	}
	l.listTop = l.inputTop + l.inputH + a.scale(10)
	l.rows = (l.footerY - a.scale(20) - l.listTop) / l.rowH
	if l.rows < 1 {
		l.rows = 1
	}
	return l
}

func (a *App) paint(c *win.Canvas) {
	if a.width == 0 {
		a.width, a.height = a.window.ClientSize()
	}
	l := a.computeLayout()

	c.Fill(win.Rect{Left: 0, Top: 0, Right: a.width, Bottom: a.height}, ColorBackground)
	a.paintBorder(c)
	// Hairline between the input and the list.
	c.Fill(win.Rect{
		Left:   l.pad,
		Top:    l.listTop - a.scale(5),
		Right:  a.width - l.pad,
		Bottom: l.listTop - a.scale(5) + 1,
	}, colorSeparator)

	a.paintQuery(c, l)
	a.paintResults(c, l)
	a.paintFooter(c, l)
}

// paintBorder draws a 1px frame so the surface reads as a window even when the
// desktop behind it is dark. Rounded corners arrive in P4.
// CornerRadius is the window's corner radius in pixels at 96 dpi. The window
// region and the painted border have to agree on it, or the border cuts across
// the arc and the corner looks chamfered.
const CornerRadius = 14

func (a *App) paintBorder(c *win.Canvas) {
	c.RoundBorder(
		win.Rect{Left: 0, Top: 0, Right: a.width, Bottom: a.height},
		a.scale(CornerRadius),
		colorBorder,
	)
}

func (a *App) paintQuery(c *win.Canvas, l layout) {
	c.UseFont(a.fonts.Query)
	textH := c.LineHeight()
	y := l.inputTop + (l.inputH-textH)/2

	caretTop := l.inputTop + a.scale(4)
	caretBottom := l.inputTop + l.inputH - a.scale(4)
	caretW := a.scale(2)
	caretGap := a.scale(5)

	// The caret marks the start of an empty field, so the placeholder begins
	// after it. Painting both at the same x put the block on top of the "S".
	if len(a.query) == 0 {
		if a.caretOn {
			c.Fill(win.Rect{Left: l.pad, Top: caretTop, Right: l.pad + caretW, Bottom: caretBottom}, colorAccent)
		}
		c.TextIn("Search applications", win.Rect{
			Left:   l.pad + caretW + caretGap,
			Top:    y,
			Right:  a.width - l.pad,
			Bottom: y + textH,
		}, colorDim)
		return
	}

	// The caret is always after the last rune, so a query wider than the field
	// shows its tail instead of being cut at the end.
	text := c.Tail(string(a.query), a.width-l.pad*2-caretW-caretGap)
	c.Text(text, l.pad, y, colorText)
	if a.caretOn {
		x := l.pad + c.Measure(text) + a.scale(1)
		c.Fill(win.Rect{Left: x, Top: caretTop, Right: x + caretW, Bottom: caretBottom}, colorAccent)
	}
}

func (a *App) paintResults(c *win.Canvas, l layout) {
	c.UseFont(a.fonts.Row)

	offset := a.listOffset(l)
	for i := int32(0); i < l.rows; i++ {
		idx := int(i) + offset
		if idx >= len(a.results) {
			break
		}

		y := l.listTop + i*l.rowH
		// Three states, one background each: the keyboard selection, the row a
		// click would hit, and everything else.
		background := ColorBackground
		switch {
		case idx == a.selected:
			background = colorSelection
			c.Fill(win.Rect{Left: 0, Top: y, Right: a.width, Bottom: y + l.rowH}, colorSelection)
			c.Fill(win.Rect{Left: 0, Top: y, Right: a.scale(3), Bottom: y + l.rowH}, colorAccent)
		case idx == a.hover:
			background = colorHover
			c.Fill(win.Rect{Left: 0, Top: y, Right: a.width, Bottom: y + l.rowH}, colorHover)
		}
		a.paintRow(c, a.results[idx], y, l, background)
	}

	if len(a.results) == 0 {
		// "No applications found" and "the index is still being built" are
		// different situations and deserve different words.
		message := "No applications found"
		if a.indexEmpty() {
			message = "Reading the index…"
		}
		c.TextIn(message, win.Rect{
			Left:   l.pad,
			Top:    l.listTop + a.scale(14),
			Right:  a.width - l.pad,
			Bottom: l.listTop + l.rowH,
		}, colorDim)
	}
}

// indexEmpty reports whether the launcher has nothing to search yet, which is
// what a cold start or a finished-but-empty scan looks like.
func (a *App) indexEmpty() bool {
	idx := a.index()
	return idx == nil || idx.Len() == 0
}

// The icon slot, and how much of it a logo may fill.
//
// The slot is the square the row reserves; the logo is drawn a little smaller
// inside it. Windows keeps that margin around every app logo, and here it also
// does optical work: a logo that is not square — Terminal and PowerShell ship
// 4:3 tiles — gets scaled to the slot's width, so filling it edge to edge makes
// the tile read as stretched next to the square logos in the same list.
const (
	iconSlot = 32 // reserved square, in logical pixels
	iconFill = 29 // drawn logo, about 90% of the slot
)

// paintRow draws one result: icon, then the name with the matched runes in the
// accent color. y is the top of the row, not of the text, and background is the
// colour the row painted, which icons with alpha need in order to composite.
func (a *App) paintRow(c *win.Canvas, r core.Result, y int32, l layout, background uint32) {
	slot := a.scale(iconSlot)
	fill := a.scale(iconFill)
	iconGap := a.scale(16)

	icon := a.icons.get(r.Entry.IconPath)
	icon.Draw(c, l.pad+(slot-fill)/2, y+(l.rowH-fill)/2, fill, background)

	// The text stays where the slot ends, so the inset never moves the column.
	x := l.pad + slot + iconGap
	textY := y + (l.rowH-c.LineHeight())/2

	name := []rune(c.Shorten(r.Entry.Name, a.width-x-l.pad))
	if len(r.Positions) == 0 {
		c.Text(string(name), x, textY, colorText)
		return
	}

	highlight := make([]bool, len(name))
	for _, p := range r.Positions {
		if p >= 0 && p < len(highlight) {
			highlight[p] = true
		}
	}

	// Draw contiguous runs so matched characters get the accent color without
	// splitting the row into one text call per character.
	for i := 0; i < len(name); {
		on := highlight[i]
		j := i
		for j < len(name) && highlight[j] == on {
			j++
		}
		segment := string(name[i:j])
		color := colorText
		if on {
			color = colorAccent
		}
		c.Text(segment, x, textY, color)
		x += c.Measure(segment)
		i = j
	}
}

func (a *App) paintFooter(c *win.Canvas, l layout) {
	c.UseFont(a.fonts.Footer)

	text, color := a.footerHint(), colorDim
	if a.status != "" {
		text, color = a.status, colorAccent
		if a.statusFailed {
			color = colorError
		}
	}
	c.TextIn(text, win.Rect{
		Left:   l.pad,
		Top:    l.footerY - a.scale(12),
		Right:  a.width - l.pad,
		Bottom: l.footerY,
	}, color)
}

// footerHint describes what the selected row can actually do. Listing
// Shift+Enter on a row that cannot be elevated, or when there are no rows at
// all, teaches the user to distrust the footer.
func (a *App) footerHint() string {
	const close = "Esc close"

	if len(a.results) == 0 {
		if len(a.query) == 0 {
			return "Type to search  ·  " + close
		}
		return "Esc clears the search  ·  F5 rescans"
	}

	entry := a.results[a.selected].Entry
	switch {
	case entry.Key == settingsKey:
		return "Enter opens the settings window  ·  " + close
	case entry.Kind() == core.KindUWP:
		// A packaged app is activated, not executed: there is no file to reveal.
		return "Enter open  ·  Shift+Enter as admin  ·  " + close
	default:
		return "Enter open  ·  Shift+Enter as admin  ·  Ctrl+Enter folder  ·  " + close
	}
}
