package ui

import (
	"image/color"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

type codeEditorRenderer struct {
	ed *codeEditor
}

func (r *codeEditorRenderer) Layout(size fyne.Size) {
	r.ed.width, r.ed.height = size.Width, size.Height
	r.ed.surface.Resize(size)
	r.ed.render(size)
}

func (r *codeEditorRenderer) MinSize() fyne.Size { return r.ed.MinSize() }
func (r *codeEditorRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.ed.surface}
}
func (r *codeEditorRenderer) Refresh() { r.ed.render(fyne.NewSize(r.ed.width, r.ed.height)) }
func (r *codeEditorRenderer) Destroy() { r.ed.stopCaret() }

func occurrenceColor() color.Color {
	c := colorAccent2
	c.A = 60
	return c
}

func selectionColor() color.Color {
	c := colorAccent
	c.A = 70
	return c
}

func matchColor(current bool) color.Color {
	c := colorWarning
	c.A = 70
	if current {
		c = colorAccent
		c.A = 110
	}
	return c
}

func severityColor(sev int) color.Color {
	if sev == 1 {
		return colorDanger
	}
	return colorWarning
}

func withAlpha(c color.NRGBA, a uint8) color.NRGBA {
	c.A = a
	return c
}

// paint collects one frame's objects.
type paint struct {
	objs []fyne.CanvasObject
}

func (p *paint) rect(c color.Color, x, y, w, h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(c)
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(w, h))
	// A layout pass can hand the editor less room than a card's margins; the
	// rasterizer panics on a stroked rectangle of negative size.
	if w <= 0 || h <= 0 {
		return r
	}
	p.objs = append(p.objs, r)
	return r
}

func (p *paint) line(c color.Color, x1, y1, x2, y2, width float32) {
	l := canvas.NewLine(c)
	l.StrokeWidth = width
	l.Position1 = fyne.NewPos(x1, y1)
	l.Position2 = fyne.NewPos(x2, y2)
	p.objs = append(p.objs, l)
}

func (p *paint) text(s string, c color.Color, size, x, y float32, style fyne.TextStyle) float32 {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle = style
	t.Move(fyne.NewPos(x, y))
	p.objs = append(p.objs, t)
	// Monospace advances are the same in every weight and slant, and not every
	// theme ships a monospace italic face to measure with.
	if style.Monospace {
		style = fyne.TextStyle{Monospace: true}
	}
	return fyne.MeasureText(s, size, style).Width
}

// drawClipped draws s at x but only the part between minX and maxX, so long
// lines and horizontal scrolling never paint outside the editor.
func (e *codeEditor) drawClipped(p *paint, s string, c color.Color, x, y, minX, maxX float32, style fyne.TextStyle) {
	size := e.textSize()
	if s == "" || x >= maxX {
		return
	}
	full := measureRunes(s, runeLen(s), size)
	if x+full <= minX {
		return
	}
	runes := []rune(s)
	start := 0
	for x < minX-1 && start < len(runes) {
		x += measureRunes(string(runes[start]), 1, size)
		start++
	}
	end := len(runes)
	if x+measureRunes(string(runes[start:]), end-start, size) > maxX {
		w := float32(0)
		for i := start; i < len(runes); i++ {
			w += measureRunes(string(runes[i]), 1, size)
			if x+w > maxX {
				end = i + 1
				break
			}
		}
	}
	if start < end {
		p.text(string(runes[start:end]), c, size, x, y, style)
	}
}

// rangeRects calls fn with the painted extent, per visible visual line, of the
// document range from..to. eol widens a line the range continues past.
func (e *codeEditor) rangeRects(from, to tpos, first, last int, eol bool, fn func(x1, x2, y float32)) {
	vls := e.layout()
	lh := e.lineHeight()
	a, b := maxInt(first, e.vlineOf(from)), minInt(last-1, e.vlineOf(to))
	for vi := a; vi <= b; vi++ {
		v := vls[vi]
		if v.row > to.row || (v.row < from.row && !v.folded) {
			continue
		}
		sc, ec := v.from, v.to
		if v.row == from.row {
			sc = maxInt(from.col, v.from)
		}
		if v.row == to.row {
			ec = minInt(to.col, v.to)
		}
		if ec < sc {
			continue
		}
		seg := e.segment(v)
		x1 := e.textX() + measureRunes(seg, sc-v.from, e.textSize())
		x2 := e.textX() + measureRunes(seg, ec-v.from, e.textSize())
		if eol && (to.row > v.row || (!v.last && to.row == v.row && to.col > v.to)) {
			x2 += e.charWidth() / 2
		}
		fn(x1, x2, 4+float32(vi)*lh-e.scrollY)
	}
}

func (e *codeEditor) render(size fyne.Size) {
	if size.Width <= 0 || size.Height <= 0 {
		size = e.MinSize()
	}
	e.width, e.height = size.Width, size.Height
	vls := e.layout()
	e.clampScroll(size.Height)
	lh := e.lineHeight()
	gw := e.gutterWidth()
	ts := e.textSize()
	cw := e.charWidth()
	minX, maxX := gw+1, size.Width-scrollbarSize
	p := &paint{objs: make([]fyne.CanvasObject, 0, 256)}
	clampX := func(x1, x2 float32) (float32, float32) { return maxFloat(minX, x1), minFloat(maxX, x2) }

	p.rect(colorSurface, 0, 0, size.Width, size.Height)

	first := int(e.scrollY / lh)
	if first < 0 {
		first = 0
	}
	last := first + int(size.Height/lh) + 2
	if last > len(vls) {
		last = len(vls)
	}
	yOf := func(vi int) float32 { return 4 + float32(vi)*lh - e.scrollY }
	sels := e.allSels()

	// Current lines: each caret without a selection.
	caretRows := map[int]bool{}
	for _, s := range sels {
		caretRows[s.head.row] = true
		if !s.empty() {
			continue
		}
		if vi := e.vlineOf(s.head); vi >= first && vi < last {
			p.rect(colorSurface2, gw, yOf(vi), size.Width-gw, lh)
		}
	}

	// Search matches, the current one stronger.
	if len(e.matches) > 0 {
		pf, pt := e.offset(e.primary().from()), e.offset(e.primary().to())
		startOff, endOff := e.offset(tpos{vls[first].row, 0}), e.offset(tpos{vls[maxInt(last-1, 0)].row, e.lineLen(vls[maxInt(last-1, 0)].row)})
		for _, m := range e.matches {
			if m.to < startOff || m.from > endOff {
				continue
			}
			c := matchColor(m.from == pf && m.to == pt)
			e.rangeRects(e.posAt(m.from), e.posAt(m.to), first, last, false, func(x1, x2, y float32) {
				x1, x2 = clampX(x1, x2)
				if x2 > x1 {
					p.rect(c, x1, y, x2-x1, lh)
				}
			})
		}
	}

	// Other occurrences of a selected word, quieter than the selection.
	if needle, row, sc, ec, ok := e.selectedOccurrence(); ok {
		nr := []rune(needle)
		for vi := first; vi < last; vi++ {
			v := vls[vi]
			if v.from != 0 {
				continue
			}
			for _, at := range findOccurrences([]rune(e.lines[v.row]), nr) {
				if v.row == row && at < ec && at+len(nr) > sc {
					continue
				}
				e.rangeRects(tpos{v.row, at}, tpos{v.row, at + len(nr)}, first, last, false, func(x1, x2, y float32) {
					x1, x2 = clampX(x1, x2)
					if x2 > x1 {
						p.rect(occurrenceColor(), x1, y, x2-x1, lh)
					}
				})
			}
		}
	}

	// Selections.
	for _, s := range sels {
		if s.empty() {
			continue
		}
		e.rangeRects(s.from(), s.to(), first, last, true, func(x1, x2, y float32) {
			x1, x2 = clampX(x1, x2)
			p.rect(selectionColor(), x1, y, maxFloat(1, x2-x1), lh)
		})
	}

	// The bracket pair at the caret.
	if e.primary().empty() {
		if at, m, ok := e.matchingBracket(tpos{e.row, e.col}); ok {
			for _, b := range []tpos{at, m} {
				vi := e.vlineOf(b)
				if vi < first || vi >= last {
					continue
				}
				x, y := e.xyOf(b)
				if x >= minX && x < maxX {
					r := p.rect(withAlpha(colorAccent, 40), x, y, measureRunes(string(e.runeAt(b.row, b.col)), 1, ts), lh)
					r.StrokeColor = withAlpha(colorAccent, 150)
					r.StrokeWidth = 1
				}
			}
		}
	}

	// Indent guides, one per level inside each line's indentation.
	unit := e.lang.unit
	guide := withAlpha(colorBorder, 200)
	for vi := first; vi < last; vi++ {
		v := vls[vi]
		ind := e.guideIndent(v.row)
		for col := 0; col < ind; col += unit {
			if v.from != 0 && col >= ind {
				break
			}
			x := e.textX() + float32(col)*cw
			if x > minX && x < maxX {
				p.rect(guide, x, yOf(vi), 1, lh)
			}
		}
	}

	// Text.
	spanCache := map[int][]hlSpan{}
	for vi := first; vi < last; vi++ {
		v := vls[vi]
		spans, ok := spanCache[v.row]
		if !ok {
			spans = e.spansAt(v.row)
			spanCache[v.row] = spans
		}
		y := yOf(vi)
		x := e.textX()
		at := 0
		for _, sp := range spans {
			n := runeLen(sp.text)
			s, t := maxInt(at, v.from), minInt(at+n, v.to)
			if s < t {
				piece := sp.text
				if s != at || t != at+n {
					r := []rune(sp.text)
					piece = string(r[s-at : t-at])
				}
				style := fyne.TextStyle{Monospace: true, Bold: sp.bold, Italic: sp.italic}
				e.drawClipped(p, piece, sp.color, x, y, minX, maxX, style)
				x += measureRunes(piece, t-s, ts)
			}
			at += n
			if at >= v.to {
				break
			}
		}
		if v.folded {
			pill := p.rect(colorSurface2, x+cw/2, y+2, cw*3, lh-4)
			pill.CornerRadius = 3
			pill.StrokeColor = colorBorder
			pill.StrokeWidth = 1
			p.text("⋯", colorMuted, ts, x+cw, y, fyne.TextStyle{Monospace: true})
		}
	}

	// Diagnostics: a wavy underline under the range.
	for _, d := range e.diags {
		if d.Line < 0 || d.Line >= len(e.lines) {
			continue
		}
		start, end := d.Col, d.EndCol
		if d.EndLine != d.Line {
			end = e.lineLen(d.Line)
		} else if end <= start {
			end = start + 1
		}
		start = maxInt(start, 0)
		if end > e.lineLen(d.Line) {
			end = e.lineLen(d.Line)
		}
		col := severityColor(d.Severity)
		draw := func(x1, x2, y float32) {
			if x2 <= x1 {
				x2 = x1 + cw
			}
			x1, x2 = clampX(x1, x2)
			step, amp := float32(3), float32(1.5)
			base := y + lh - 3
			up := true
			for sx := x1; sx < x2; sx += step {
				w := minFloat(step, x2-sx)
				y1, y2 := base, base+amp
				if !up {
					y1, y2 = base+amp, base
				}
				p.line(col, sx, y1, sx+w, y2, 1.2)
				up = !up
			}
		}
		if start >= end {
			if vi := e.vlineOf(tpos{d.Line, start}); vi >= first && vi < last {
				x, y := e.xyOf(tpos{d.Line, start})
				draw(x, x+cw, y)
			}
			continue
		}
		e.rangeRects(tpos{d.Line, start}, tpos{d.Line, end}, first, last, false, draw)
	}

	if ghost, gx, gy, ok := e.ghostText(); ok {
		gc := colorFaint
		gc.A = 190
		e.drawClipped(p, ghost, gc, gx, gy, minX, maxX, fyne.TextStyle{Monospace: true, Italic: true})
	}

	// Carets.
	if e.focused && e.caretOn {
		for _, s := range sels {
			vi := e.vlineOf(s.head)
			if vi < first || vi >= last {
				continue
			}
			x, y := e.xyOf(s.head)
			if x >= minX-1 && x < maxX {
				p.rect(colorAccent, x, y, 2, lh-1)
			}
		}
	}

	// The gutter goes on top, so text scrolled sideways slides under it.
	p.rect(colorSurface2, 0, 0, gw, size.Height)
	p.rect(colorBorder, gw, 0, 1, size.Height)
	cs := e.captionSize()
	for vi := first; vi < last; vi++ {
		v := vls[vi]
		if v.from != 0 {
			continue
		}
		y := yOf(vi)
		numColor := colorFaint
		if caretRows[v.row] {
			numColor = colorText
		}
		num := itoa(v.row + 1)
		nw := fyne.MeasureText(num, cs, fyne.TextStyle{Monospace: true}).Width
		p.text(num, numColor, cs, gw-foldColumn-4-nw, y+(lh-cs)/2-2, fyne.TextStyle{Monospace: true})
		if v.folded {
			p.text("▸", colorAccent, cs, gw-foldColumn+1, y+(lh-cs)/2-2, fyne.TextStyle{})
		} else if _, ok := e.foldEnd(v.row); ok {
			p.text("▾", withAlpha(colorFaint, 170), cs, gw-foldColumn+1, y+(lh-cs)/2-2, fyne.TextStyle{})
		}
	}
	for _, d := range e.diags {
		if d.Line < 0 || d.Line >= len(e.lines) {
			continue
		}
		vi := e.vlineOf(tpos{d.Line, 0})
		if vi >= first && vi < last {
			p.rect(severityColor(d.Severity), 2, yOf(vi), 3, lh)
		}
	}

	e.paintScrollbars(p, size, vls)

	if e.hoverVisible && !e.comp.visible {
		e.renderHover(p, size, lh)
	}
	if e.comp.visible {
		e.renderCompletion(p, size, gw, lh)
	}
	e.surface.Objects = p.objs
	e.surface.Refresh()
}

func minFloat(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// guideIndent is how deep the indent guides go on row: its indentation, or
// for a blank line the shallower of its neighbours'.
func (e *codeEditor) guideIndent(row int) int {
	if strings.TrimSpace(e.lines[row]) != "" {
		return leadingSpaces(e.lines[row])
	}
	up, down := 0, 0
	for r := row - 1; r >= 0 && r > row-200; r-- {
		if strings.TrimSpace(e.lines[r]) != "" {
			up = leadingSpaces(e.lines[r])
			break
		}
	}
	for r := row + 1; r < len(e.lines) && r < row+200; r++ {
		if strings.TrimSpace(e.lines[r]) != "" {
			down = leadingSpaces(e.lines[r])
			break
		}
	}
	return minInt(up, down)
}

// paintScrollbars draws the vertical thumb with an overview of matches,
// diagnostics and carets, and a horizontal thumb when lines are wider than
// the view.
func (e *codeEditor) paintScrollbars(p *paint, size fyne.Size, vls []vline) {
	x := size.Width - scrollbarSize
	total := float32(len(vls))
	tick := func(row int, c color.Color) {
		if total <= 0 {
			return
		}
		y := float32(e.vlineOf(tpos{row, 0})) / total * size.Height
		p.rect(c, x+1, y, scrollbarSize-2, 2)
	}
	if e.maxScrollY(size.Height) > 0 || len(e.matches) > 0 || len(e.diags) > 0 {
		p.rect(withAlpha(colorBorder, 60), x, 0, 1, size.Height)
	}
	for i, m := range e.matches {
		if i > 2000 {
			break
		}
		tick(e.posAt(m.from).row, withAlpha(colorWarning, 200))
	}
	for _, d := range e.diags {
		tick(d.Line, severityColor(d.Severity))
	}
	if e.maxScrollY(size.Height) > 0 {
		top, h := e.thumb()
		a := uint8(70)
		if e.dragMode == dragModeBar {
			a = 130
		}
		th := p.rect(withAlpha(colorFaint, a), x+2, top, scrollbarSize-4, h)
		th.CornerRadius = 3
	}
	tick(e.row, withAlpha(colorText, 160))
	if mx := e.maxScrollX(); mx > 0 {
		viewW := size.Width - e.gutterWidth() - scrollbarSize
		w := viewW * viewW / (viewW + mx)
		if w < 24 {
			w = 24
		}
		left := e.gutterWidth() + (viewW-w)*e.scrollX/mx
		th := p.rect(withAlpha(colorFaint, 70), left, size.Height-6, w, 4)
		th.CornerRadius = 2
	}
}

// selectedOccurrence reports a single-line, non-trivial primary selection
// whose other occurrences should be highlighted.
func (e *codeEditor) selectedOccurrence() (text string, row, sc, ec int, ok bool) {
	sr, sc, er, ec, ok := e.selection()
	if !ok || sr != er || e.multi() {
		return "", 0, 0, 0, false
	}
	text = e.selectionText()
	if utf8.RuneCountInString(text) < 2 || strings.TrimSpace(text) == "" {
		return "", 0, 0, 0, false
	}
	return text, sr, sc, ec, true
}

func findOccurrences(line, needle []rune) []int {
	if len(needle) == 0 || len(needle) > len(line) {
		return nil
	}
	var out []int
	for i := 0; i+len(needle) <= len(line); i++ {
		if runesAt(line, needle, i) {
			out = append(out, i)
		}
	}
	return out
}

// ghostText is the rest of the selected completion, drawn after the caret.
func (e *codeEditor) ghostText() (string, float32, float32, bool) {
	if !e.comp.visible || len(e.comp.items) == 0 {
		return "", 0, 0, false
	}
	it := e.comp.items[e.comp.selected]
	text := stripSnippet(it.Insert)
	if text == "" {
		text = it.Label
	}
	if !strings.HasPrefix(strings.ToLower(text), strings.ToLower(e.comp.prefix)) {
		return "", 0, 0, false
	}
	rest := text[len(e.comp.prefix):]
	if rest == "" {
		return "", 0, 0, false
	}
	x, y := e.xyOf(tpos{e.row, e.col})
	return rest, x, y, true
}

func (e *codeEditor) renderHover(p *paint, size fyne.Size, lh float32) {
	lines := wrapHover(e.hoverText, 58)
	if len(lines) == 0 {
		return
	}
	textSize := maxFloat(e.captionSize()-1, 9)
	lineH := textSize + 3
	padX, padY := float32(7), float32(4)
	width := float32(180)
	for _, line := range lines {
		if w := fyne.MeasureText(line, textSize, fyne.TextStyle{Monospace: true}).Width + padX*2; w > width {
			width = w
		}
	}
	width = minFloat(minFloat(width, 460), size.Width-24)
	height := float32(len(lines))*lineH + padY*2
	cx, cy := e.xyOf(tpos{e.row, e.col})
	x := cx
	if x+width > size.Width-8 {
		x = size.Width - width - 8
	}
	if x < e.gutterWidth()+8 {
		x = e.gutterWidth() + 8
	}
	y := cy + lh + 4
	if y+height > size.Height-8 {
		y = cy - height - 4
	}
	if y < 8 {
		y = 8
	}
	bg := p.rect(colorFloat, x, y, width, height)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	bg.CornerRadius = 3
	for i, line := range lines {
		p.text(line, colorText, textSize, x+padX, y+padY+float32(i)*lineH, fyne.TextStyle{Monospace: true})
	}
}

// wrapHover wraps a hover message without truncating it. The first line is a
// compact heading; subsequent lines are messages/bullets.
func wrapHover(text string, width int) []string {
	var out []string
	for _, raw := range strings.Split(strings.TrimSpace(text), "\n") {
		words := strings.Fields(raw)
		if len(words) == 0 {
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len([]rune(line))+1+len([]rune(w)) > width {
				out = append(out, line)
				line = w
				continue
			}
			line += " " + w
		}
		out = append(out, line)
	}
	return out
}

func (e *codeEditor) pointInCompletion(p fyne.Position) bool {
	return e.comp.visible && p.X >= e.comp.x && p.X <= e.comp.x+e.comp.w &&
		p.Y >= e.comp.y && p.Y <= e.comp.y+e.comp.h
}

func (e *codeEditor) completionIndexAt(p fyne.Position) (int, bool) {
	if !e.pointInCompletion(p) {
		return 0, false
	}
	idx := e.comp.scroll + int((p.Y-(e.comp.y+3))/e.comp.rowH)
	if idx < e.comp.scroll || idx >= e.comp.scroll+e.comp.rows || idx >= len(e.comp.items) {
		return 0, false
	}
	return idx, true
}

func (e *codeEditor) renderCompletion(p *paint, size fyne.Size, gw, lh float32) {
	if len(e.comp.items) == 0 {
		return
	}
	width := minFloat(330, size.Width-24)
	rowH := float32(20)
	maxRows := 10
	if avail := int((size.Height - 28) / rowH); avail < maxRows {
		maxRows = avail
	}
	if maxRows < 1 {
		maxRows = 1
	}
	rows := minInt(len(e.comp.items), maxRows)
	e.comp.rows = rows
	if e.comp.scroll > len(e.comp.items)-rows {
		e.comp.scroll = len(e.comp.items) - rows
	}
	if e.comp.scroll < 0 {
		e.comp.scroll = 0
	}
	height := float32(rows)*rowH + 6
	ax, ay := e.xyOf(tpos{e.comp.row, e.comp.col})
	x := ax
	if x+width > size.Width-8 {
		x = size.Width - width - 8
	}
	if x < gw+8 {
		x = gw + 8
	}
	y := ay + lh
	if y+height > size.Height-8 {
		y = ay - height - 2
	}
	if y < 8 {
		y = 8
	}
	e.comp.x, e.comp.y, e.comp.w, e.comp.h, e.comp.rowH = x, y, width, height, rowH

	bg := p.rect(colorFloat, x, y, width, height)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	bg.CornerRadius = 3

	caption := maxFloat(e.captionSize()-1, 9)
	mono := fyne.TextStyle{Monospace: true}
	for visible := 0; visible < rows; visible++ {
		idx := e.comp.scroll + visible
		if idx >= len(e.comp.items) {
			break
		}
		it := e.comp.items[idx]
		rowY := y + 3 + float32(visible)*rowH
		if idx == e.comp.selected {
			p.rect(selectionColor(), x+3, rowY, width-6, rowH)
			p.rect(colorAccent, x+3, rowY, 2, rowH)
		}
		icon := "·"
		if strings.Contains(it.Detail, "(") {
			icon = "ƒ"
		}
		p.text(icon, colorAccent, caption, x+7, rowY+2, mono)
		label := oneLineText(it.Label, 36)
		lw := p.text(label, colorText, caption, x+19, rowY+2, mono)
		if it.Detail != "" {
			p.text(oneLineText(it.Detail, 34), colorMuted, caption, x+19+lw+10, rowY+2, mono)
		}
	}
	if len(e.comp.items) > rows {
		p.rect(colorBorder, x+width-4, y+3, 2, height-6)
		thumbH := maxFloat((height-6)*float32(rows)/float32(len(e.comp.items)), 12)
		maxScroll := len(e.comp.items) - rows
		frac := float32(0)
		if maxScroll > 0 {
			frac = float32(e.comp.scroll) / float32(maxScroll)
		}
		p.rect(colorFaint, x+width-4, y+3+frac*(height-6-thumbH), 2, thumbH)
	}
}
