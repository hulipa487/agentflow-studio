package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// editorKeys documents the bindings; the context menu and the docs show it.
var editorKeys = [][2]string{
	{"Ctrl+F / Ctrl+H", "Find / replace (F3, Shift+F3 for the next and previous match)"},
	{"Ctrl+G", "Go to line"},
	{"Ctrl+D", "Select the word, then add its next occurrence"},
	{"Ctrl+Shift+L", "Select every occurrence"},
	{"Ctrl+Alt+Up/Down", "Add a caret above or below"},
	{"Alt+click, Alt+drag", "Add a caret, select a column"},
	{"Ctrl+/", "Toggle comment"},
	{"Alt+Up/Down", "Move lines"},
	{"Shift+Alt+Up/Down", "Copy lines up or down"},
	{"Ctrl+Shift+K", "Delete lines"},
	{"Ctrl+J", "Join lines"},
	{"Ctrl+L", "Select the line"},
	{"Ctrl+] / Ctrl+[", "Indent / outdent"},
	{"Ctrl+U / Ctrl+Shift+U", "Lower / upper case"},
	{"Ctrl+Shift+[ / ]", "Fold / unfold"},
	{"Ctrl+Shift+\\", "Go to the matching bracket"},
	{"Alt+Z", "Toggle word wrap"},
	{"Ctrl+= / Ctrl+- / Ctrl+0", "Zoom in, out, reset (also Ctrl+wheel)"},
}

// TypedRune is called for printable input while the editor is focused.
func (e *codeEditor) TypedRune(r rune) {
	// Left Alt chords are commands, not text; right Alt is AltGr, which
	// types characters such as @ { [ on many layouts.
	if e.altDown && !e.ctrlDown {
		return
	}
	e.insert(r)
}

func (e *codeEditor) KeyDown(key *fyne.KeyEvent) {
	switch key.Name {
	case desktop.KeyShiftLeft, desktop.KeyShiftRight:
		e.shiftDown = true
	case desktop.KeyControlLeft, desktop.KeyControlRight:
		e.ctrlDown = true
	case desktop.KeyAltLeft:
		e.altDown = true
	}
}

func (e *codeEditor) KeyUp(key *fyne.KeyEvent) {
	switch key.Name {
	case desktop.KeyShiftLeft, desktop.KeyShiftRight:
		e.shiftDown = false
	case desktop.KeyControlLeft, desktop.KeyControlRight:
		e.ctrlDown = false
	case desktop.KeyAltLeft:
		e.altDown = false
	}
}

func (e *codeEditor) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		switch {
		case e.comp.visible:
			e.comp.visible = false
			e.Refresh()
		case e.collapseToPrimary():
		case e.sel:
			e.setSels([]selRange{caretAt(tpos{e.row, e.col})}, 0)
			e.Refresh()
		case e.onShortcut != nil:
			e.onShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape})
		}
	case fyne.KeyUp:
		if e.comp.visible {
			if e.comp.selected > 0 {
				e.comp.selected--
				e.scrollCompletion()
				e.Refresh()
			}
			return
		}
		e.move(-1, 0, e.shiftDown)
	case fyne.KeyDown:
		if e.comp.visible {
			if e.comp.selected < len(e.comp.items)-1 {
				e.comp.selected++
				e.scrollCompletion()
				e.Refresh()
			}
			return
		}
		e.move(1, 0, e.shiftDown)
	case fyne.KeyLeft:
		e.move(0, -1, e.shiftDown)
	case fyne.KeyRight:
		e.move(0, 1, e.shiftDown)
	case fyne.KeyHome:
		e.home(e.shiftDown)
	case fyne.KeyEnd:
		e.end(e.shiftDown)
	case fyne.KeyPageUp:
		e.page(-1, e.shiftDown)
	case fyne.KeyPageDown:
		e.page(1, e.shiftDown)
	case fyne.KeyBackspace:
		e.backspace()
	case fyne.KeyDelete:
		e.deleteForward()
	case fyne.KeyReturn, fyne.KeyEnter:
		if e.comp.visible {
			e.applyCompletion()
			return
		}
		e.insert('\n')
	case fyne.KeyTab:
		if e.comp.visible {
			e.applyCompletion()
			return
		}
		e.indent(e.shiftDown)
	case fyne.KeyF3:
		e.findNext(e.shiftDown)
	}
}

func (e *codeEditor) scrollCompletion() {
	if !e.comp.visible || e.comp.rows <= 0 {
		return
	}
	if e.comp.selected < e.comp.scroll {
		e.comp.scroll = e.comp.selected
	}
	if e.comp.selected >= e.comp.scroll+e.comp.rows {
		e.comp.scroll = e.comp.selected - e.comp.rows + 1
	}
	if e.comp.scroll < 0 {
		e.comp.scroll = 0
	}
}

func (e *codeEditor) TypedShortcut(s fyne.Shortcut) {
	switch s.(type) {
	case *fyne.ShortcutSelectAll:
		e.selectAll()
		return
	case *fyne.ShortcutCopy:
		e.copySelection()
		return
	case *fyne.ShortcutCut:
		e.cutSelection()
		return
	case *fyne.ShortcutPaste:
		e.paste()
		return
	case *fyne.ShortcutUndo:
		e.undo()
		return
	case *fyne.ShortcutRedo:
		e.redo()
		return
	}
	cs, ok := s.(*desktop.CustomShortcut)
	if !ok {
		return
	}
	if e.handleChord(cs) {
		return
	}
	if e.onShortcut != nil {
		e.onShortcut(cs)
	}
}

// handleChord runs the editor's own modifier bindings; false passes the
// shortcut on to the window (Ctrl+S, Ctrl+Enter, navigation).
func (e *codeEditor) handleChord(cs *desktop.CustomShortcut) bool {
	ctrl := cs.Modifier&fyne.KeyModifierControl != 0 || cs.Modifier&fyne.KeyModifierSuper != 0
	shift := cs.Modifier&fyne.KeyModifierShift != 0
	alt := cs.Modifier&fyne.KeyModifierAlt != 0
	k := cs.KeyName
	switch {
	case ctrl && alt && !shift:
		switch k {
		case fyne.KeyUp:
			e.addCaretVertical(-1)
		case fyne.KeyDown:
			e.addCaretVertical(1)
		default:
			return false
		}
		return true
	case alt && shift && !ctrl:
		switch k {
		case fyne.KeyUp:
			e.copyLines(false)
		case fyne.KeyDown:
			e.copyLines(true)
		default:
			return false
		}
		return true
	case alt && !ctrl && !shift:
		switch k {
		case fyne.KeyUp:
			e.moveLines(-1)
		case fyne.KeyDown:
			e.moveLines(1)
		case fyne.KeyZ:
			e.SetWrap(!e.wrap)
		default:
			return false
		}
		return true
	case ctrl && shift && !alt:
		switch k {
		case fyne.KeyZ:
			e.redo()
		case fyne.KeyK:
			e.deleteLines()
		case fyne.KeyL:
			e.selectAllOccurrences()
		case fyne.KeyU:
			e.changeCase(true)
		case fyne.KeyD:
			e.copyLines(true)
		case fyne.KeyLeftBracket:
			e.foldAtCaret()
		case fyne.KeyRightBracket:
			e.unfoldAtCaret()
		case fyne.KeyBackslash:
			e.jumpToBracket()
		case fyne.KeyLeft:
			e.wordMove(true, true)
		case fyne.KeyRight:
			e.wordMove(false, true)
		case fyne.KeyHome:
			e.docEdge(false, true)
		case fyne.KeyEnd:
			e.docEdge(true, true)
		case fyne.KeyF3:
			e.findNext(true)
		default:
			return false
		}
		return true
	case ctrl && !shift && !alt:
		switch k {
		case fyne.KeyZ:
			e.undo()
		case fyne.KeyY:
			e.redo()
		case fyne.KeyA:
			e.selectAll()
		case fyne.KeyC:
			e.copySelection()
		case fyne.KeyX:
			e.cutSelection()
		case fyne.KeyV:
			e.paste()
		case fyne.KeySpace:
			e.RequestCompletion(true)
		case fyne.KeyBackspace:
			e.deleteWordLeft()
		case fyne.KeyDelete:
			e.deleteWordRight()
		case fyne.KeyLeft:
			e.wordMove(true, false)
		case fyne.KeyRight:
			e.wordMove(false, false)
		case fyne.KeyHome:
			e.docEdge(false, false)
		case fyne.KeyEnd:
			e.docEdge(true, false)
		case fyne.KeyD:
			e.addNextOccurrence()
		case fyne.KeySlash:
			e.toggleComment()
		case fyne.KeyJ:
			e.joinLines()
		case fyne.KeyL:
			e.expandLineSelection()
		case fyne.KeyU:
			e.changeCase(false)
		case fyne.KeyRightBracket:
			e.shiftLines(false)
		case fyne.KeyLeftBracket:
			e.shiftLines(true)
		case fyne.KeyF:
			if e.onFind != nil {
				e.onFind(false)
			}
		case fyne.KeyH:
			if e.onFind != nil {
				e.onFind(true)
			}
		case fyne.KeyG:
			if e.onGoto != nil {
				e.onGoto()
			}
		case fyne.KeyEqual, fyne.KeyPlus:
			e.zoom(1)
		case fyne.KeyMinus:
			e.zoom(-1)
		case fyne.Key0:
			e.zoom(0)
		default:
			return false
		}
		return true
	case shift && !ctrl && !alt:
		switch k {
		case fyne.KeyLeft:
			e.move(0, -1, true)
		case fyne.KeyRight:
			e.move(0, 1, true)
		case fyne.KeyUp:
			e.move(-1, 0, true)
		case fyne.KeyDown:
			e.move(1, 0, true)
		case fyne.KeyHome:
			e.home(true)
		case fyne.KeyEnd:
			e.end(true)
		case fyne.KeyPageUp:
			e.page(-1, true)
		case fyne.KeyPageDown:
			e.page(1, true)
		case fyne.KeyTab:
			e.indent(true)
		case fyne.KeyF3:
			e.findNext(true)
		default:
			return false
		}
		return true
	}
	return false
}

// zoom steps the shared editor text size; 0 resets it.
func (e *codeEditor) zoom(dir int) {
	switch {
	case dir > 0 && editorZoom < 2.5:
		editorZoom += 0.1
	case dir < 0 && editorZoom > 0.6:
		editorZoom -= 0.1
	case dir == 0:
		editorZoom = 1
	}
	e.invalidateLayout()
	e.revealCursor()
	e.Refresh()
}

// ---- mouse ------------------------------------------------------------------

const (
	dragModeText = iota
	dragModeColumn
	dragModeBar
	dragModeGutter
)

func (e *codeEditor) MouseDown(m *desktop.MouseEvent) {
	if m.Button != desktop.MouseButtonPrimary {
		return
	}
	e.touchCaret()
	if e.comp.visible {
		if idx, ok := e.completionIndexAt(m.Position); ok {
			e.comp.selected = idx
			e.applyCompletion()
			return
		}
	}
	if c := e.canvas(); c != nil {
		c.Focus(e)
	}
	e.comp.visible = false
	e.hoverVisible = false
	// The vertical scrollbar.
	if m.Position.X >= e.viewWidth()-scrollbarSize && e.maxScrollY(e.viewHeight()) > 0 {
		e.dragMode = dragModeBar
		top, h := e.thumb()
		if m.Position.Y < top || m.Position.Y > top+h {
			e.scrollToThumb(m.Position.Y - h/2)
			top, _ = e.thumb()
		}
		e.dragBarGrab = m.Position.Y - top
		e.Refresh()
		return
	}
	row, col := e.posToCursor(m.Position)
	// The gutter: the fold column toggles a fold, the numbers select lines.
	if m.Position.X < e.gutterWidth() {
		if m.Position.X >= e.gutterWidth()-foldColumn-2 {
			if _, ok := e.foldEnd(row); ok || e.folds[row] {
				e.toggleFold(row)
				return
			}
		}
		e.dragMode = dragModeGutter
		e.dragAnchor = tpos{row, 0}
		e.selectLine(row)
		return
	}
	p := tpos{row, col}
	alt := m.Modifier&fyne.KeyModifierAlt != 0
	shift := m.Modifier&fyne.KeyModifierShift != 0
	switch {
	case alt && shift:
		e.dragMode = dragModeColumn
		e.dragAnchor = tpos{e.row, e.col}
		e.columnSelect(e.dragAnchor, p)
	case alt:
		// Alt+click adds a caret, or removes one already there.
		sels := e.allSels()
		for i, s := range sels {
			if s.empty() && s.head == p && len(sels) > 1 {
				sels = append(sels[:i], sels[i+1:]...)
				e.setSels(sels, 0)
				e.cursorMoved()
				return
			}
		}
		e.setSels(append(sels, caretAt(p)), len(sels))
		e.dragMode = dragModeColumn
		e.dragAnchor = p
	case shift:
		e.dragMode = dragModeText
		e.setSels([]selRange{{anchor: tpos{e.anchorR, e.anchorC}, head: p, goal: -1}}, 0)
		if !e.sel {
			e.setSels([]selRange{{anchor: tpos{e.row, e.col}, head: p, goal: -1}}, 0)
		}
	default:
		e.dragMode = dragModeText
		// A third click right after a double-click selects the line.
		if time.Since(e.lastDouble) < 450*time.Millisecond && e.lastDoubleAt == row {
			e.lastDouble = time.Time{}
			e.selectLine(row)
			e.dragAnchor = tpos{row, 0}
			e.dragMode = dragModeGutter
			return
		}
		e.setSels([]selRange{caretAt(p)}, 0)
		e.dragAnchor = p
	}
	e.revealCursor()
	e.cursorMoved()
}

func (e *codeEditor) MouseUp(*desktop.MouseEvent) {}

// overDiagnostic checks the actual painted squiggle, not just the column. A
// pointer in the gap before a glyph maps to that glyph's column but is not
// over the red line, so it must not open the error card.
func (e *codeEditor) overDiagnostic(row, col int, x float32) bool {
	if row < 0 || row >= len(e.lines) {
		return false
	}
	n := e.lineLen(row)
	for _, d := range e.diags {
		if d.Line != row {
			continue
		}
		start, end := d.Col, d.EndCol
		if d.EndLine != row {
			end = n
		} else if end <= start {
			end = start + 1
		}
		start, end = maxInt(start, 0), minInt(end, n)
		if col < start || col >= end {
			continue
		}
		x1, _ := e.xyOf(tpos{row, start})
		x2, _ := e.xyOf(tpos{row, end})
		if end == n {
			x2 = x1 + measureRunes(string([]rune(e.lines[row])[start:end]), end-start, e.textSize())
		}
		if x >= x1 && x < x2 {
			return true
		}
	}
	return false
}

func (e *codeEditor) MouseIn(*desktop.MouseEvent) {}

func (e *codeEditor) MouseMoved(m *desktop.MouseEvent) {
	if m == nil || e.onHover == nil {
		return
	}
	if m.Position.X < e.gutterWidth() || m.Position.X >= e.viewWidth()-scrollbarSize {
		return
	}
	row, col := e.posToCursor(m.Position)
	if row == e.hoverRow && col == e.hoverCol {
		return
	}
	e.hoverRow, e.hoverCol = row, col
	e.onHover(row, col, e.overDiagnostic(row, col, m.Position.X))
}

func (e *codeEditor) MouseOut() {
	if e.onHoverOut != nil {
		e.onHoverOut()
	}
}

// DoubleTapped selects the identifier (or punctuation/whitespace run) under
// the pointer. A click past the end of the line's text selects the whole line
// instead, because there is no word out there to take.
func (e *codeEditor) DoubleTapped(p *fyne.PointEvent) {
	if p == nil || p.Position.X < e.gutterWidth() {
		return
	}
	if c := e.canvas(); c != nil {
		c.Focus(e)
	}
	row, col := e.posToCursor(p.Position)
	e.lastDouble, e.lastDoubleAt = time.Now(), row
	if e.pastLineEnd(row, p.Position.X) {
		e.selectLine(row)
		return
	}
	e.selectWord(row, col)
}

func (e *codeEditor) Dragged(d *fyne.DragEvent) {
	e.touchCaret()
	if e.dragMode == dragModeBar {
		e.scrollToThumb(d.Position.Y - e.dragBarGrab)
		e.Refresh()
		return
	}
	// Dragging past the top or bottom edge scrolls.
	lh := e.lineHeight()
	if d.Position.Y < 0 {
		e.scrollY -= lh
	} else if d.Position.Y > e.viewHeight() {
		e.scrollY += lh
	}
	e.clampScroll(e.viewHeight())
	row, col := e.posToCursor(d.Position)
	p := tpos{row, col}
	switch e.dragMode {
	case dragModeColumn:
		e.columnSelect(e.dragAnchor, p)
	case dragModeGutter:
		end := tpos{row + 1, 0}
		if row+1 >= len(e.lines) {
			end = tpos{row, e.lineLen(row)}
		}
		if row < e.dragAnchor.row {
			e.setSels([]selRange{{anchor: tpos{e.dragAnchor.row + 1, 0}, head: tpos{row, 0}, goal: -1}}, 0)
		} else {
			e.setSels([]selRange{{anchor: e.dragAnchor, head: end, goal: -1}}, 0)
		}
	default:
		sels := e.allSels()
		sels[0].head = p
		sels[0].goal = -1
		e.setSels(sels, 0)
	}
	e.Refresh()
}

func (e *codeEditor) DragEnd() { e.dragMode = dragModeText }

func (e *codeEditor) Scrolled(s *fyne.ScrollEvent) {
	e.touchCaret()
	if e.comp.visible && e.pointInCompletion(s.Position) {
		if s.Scrolled.DY < 0 {
			e.comp.scroll++
		} else if s.Scrolled.DY > 0 && e.comp.scroll > 0 {
			e.comp.scroll--
		}
		maxScroll := len(e.comp.items) - e.comp.rows
		if maxScroll < 0 {
			maxScroll = 0
		}
		if e.comp.scroll > maxScroll {
			e.comp.scroll = maxScroll
		}
		e.Refresh()
		return
	}
	if e.ctrlDown && s.Scrolled.DY != 0 {
		if s.Scrolled.DY > 0 {
			e.zoom(1)
		} else {
			e.zoom(-1)
		}
		return
	}
	dx, dy := s.Scrolled.DX, s.Scrolled.DY
	if e.shiftDown && dx == 0 {
		dx, dy = dy, 0
	}
	if dx != 0 {
		e.scrollX -= dx * 3
		if mx := e.maxScrollX(); e.scrollX > mx {
			e.scrollX = mx
		}
	}
	if dy != 0 {
		e.scrollY -= dy * e.lineHeight() / 4
	}
	e.clampScroll(e.viewHeight())
	e.comp.visible = false
	e.hoverVisible = false
	e.Refresh()
}

// thumb is the vertical scrollbar thumb's top and height.
func (e *codeEditor) thumb() (float32, float32) {
	viewH := e.viewHeight()
	content := e.contentHeight()
	if content <= viewH {
		return 0, viewH
	}
	h := viewH * viewH / content
	if h < 24 {
		h = 24
	}
	top := (viewH - h) * e.scrollY / e.maxScrollY(viewH)
	return top, h
}

func (e *codeEditor) scrollToThumb(top float32) {
	viewH := e.viewHeight()
	_, h := e.thumb()
	if viewH-h <= 0 {
		return
	}
	e.scrollY = top / (viewH - h) * e.maxScrollY(viewH)
	e.clampScroll(viewH)
}

// TappedSecondary opens the editor's context menu.
func (e *codeEditor) TappedSecondary(p *fyne.PointEvent) {
	c := e.canvas()
	if c == nil {
		return
	}
	c.Focus(e)
	item := func(label string, fn func()) *fyne.MenuItem { return fyne.NewMenuItem(label, fn) }
	items := []*fyne.MenuItem{
		item("Cut", e.cutSelection),
		item("Copy", e.copySelection),
		item("Paste", e.paste),
		item("Select all", e.selectAll),
		fyne.NewMenuItemSeparator(),
		item("Find…", func() {
			if e.onFind != nil {
				e.onFind(false)
			}
		}),
		item("Replace…", func() {
			if e.onFind != nil {
				e.onFind(true)
			}
		}),
		item("Go to line…", func() {
			if e.onGoto != nil {
				e.onGoto()
			}
		}),
		fyne.NewMenuItemSeparator(),
		item("Select every occurrence", e.selectAllOccurrences),
	}
	if e.lang.lineComment != "" || e.lang.blockComment[0] != "" {
		items = append(items, item("Toggle comment", e.toggleComment))
	}
	items = append(items,
		item("Fold all", e.foldAll),
		item("Unfold all", e.unfoldAll),
	)
	wrap := item("Word wrap", func() { e.SetWrap(!e.wrap) })
	wrap.Checked = e.wrap
	items = append(items, wrap)
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), c, p.AbsolutePosition)
}
