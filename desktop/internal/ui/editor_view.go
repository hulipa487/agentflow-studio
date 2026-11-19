package ui

import (
	"strings"

	"fyne.io/fyne/v2"
)

// vline is one visual line: the runes [from, to) of a document row. A row is
// one vline unless soft wrap splits it; the rows a fold hides have none, and
// the fold's first row is marked folded.
type vline struct {
	row, from, to int
	folded        bool
	last          bool // the last segment of its row
}

type layoutKey struct {
	version, foldGen int
	wrap             bool
	cols             int
}

const foldColumn = 14

func (e *codeEditor) invalidateLayout() { e.vls = nil }

// gutterWidth is the line numbers plus the fold column.
func (e *codeEditor) gutterWidth() float32 {
	digits := len(itoa(len(e.lines)))
	if digits < 2 {
		digits = 2
	}
	return float32(digits)*e.charWidth() + 14 + foldColumn
}

func (e *codeEditor) textX() float32 { return e.gutterWidth() + 8 - e.scrollX }

func (e *codeEditor) viewWidth() float32 {
	if e.width > 0 {
		return e.width
	}
	return e.MinSize().Width
}

func (e *codeEditor) viewHeight() float32 {
	if e.height > 0 {
		return e.height
	}
	return e.MinSize().Height
}

// wrapCols is how many columns fit before soft wrap breaks a line; 0 when
// wrapping is off.
func (e *codeEditor) wrapCols() int {
	if !e.wrap {
		return 0
	}
	cols := int((e.viewWidth() - e.gutterWidth() - 8 - scrollbarSize - 8) / e.charWidth())
	if cols < 20 {
		cols = 20
	}
	return cols
}

// layout returns the visual lines, rebuilding them when the text, the folds or
// the wrap width changed.
func (e *codeEditor) layout() []vline {
	key := layoutKey{e.version, e.foldGen, e.wrap, e.wrapCols()}
	if e.vls != nil && key == e.layoutKey && len(e.firstV) == len(e.lines) {
		return e.vls
	}
	e.layoutKey = key
	e.vls = e.vls[:0]
	if cap(e.firstV) >= len(e.lines) {
		e.firstV = e.firstV[:len(e.lines)]
	} else {
		e.firstV = make([]int, len(e.lines))
	}
	for row := 0; row < len(e.lines); row++ {
		e.firstV[row] = len(e.vls)
		n := e.lineLen(row)
		if e.folds[row] {
			if end, ok := e.foldEnd(row); ok {
				e.vls = append(e.vls, vline{row: row, from: 0, to: n, folded: true, last: true})
				for r := row + 1; r <= end; r++ {
					e.firstV[r] = -1
				}
				row = end
				continue
			}
			delete(e.folds, row)
		}
		if key.cols == 0 || n <= key.cols {
			e.vls = append(e.vls, vline{row: row, from: 0, to: n, last: true})
			continue
		}
		line := []rune(e.lines[row])
		pos := 0
		for n-pos > key.cols {
			brk := pos + key.cols
			for i := brk; i > pos+key.cols/2; i-- {
				if line[i-1] == ' ' {
					brk = i
					break
				}
			}
			e.vls = append(e.vls, vline{row: row, from: pos, to: brk})
			pos = brk
		}
		e.vls = append(e.vls, vline{row: row, from: pos, to: n, last: true})
	}
	if len(e.vls) == 0 {
		e.vls = append(e.vls, vline{row: 0, last: true})
	}
	return e.vls
}

// vlineOf returns the index of the visual line showing p. A position in a
// folded region maps to the fold's first line.
func (e *codeEditor) vlineOf(p tpos) int {
	vls := e.layout()
	p = e.clampPos(p)
	row := p.row
	for row > 0 && e.firstV[row] < 0 {
		row--
	}
	i := e.firstV[row]
	if i < 0 {
		return 0
	}
	if row != p.row || vls[i].folded {
		return i
	}
	for i < len(vls) && vls[i].row == row {
		v := vls[i]
		if p.col < v.to || v.last {
			return i
		}
		i++
	}
	return len(vls) - 1
}

// segment returns the text of a visual line.
func (e *codeEditor) segment(v vline) string {
	if v.from == 0 && v.last {
		return e.lines[v.row]
	}
	r := []rune(e.lines[v.row])
	return string(r[v.from:minInt(v.to, len(r))])
}

// xyOf returns where p is drawn: the left edge of its glyph and the top of its
// line, in the editor's coordinates.
func (e *codeEditor) xyOf(p tpos) (float32, float32) {
	vi := e.vlineOf(p)
	v := e.layout()[vi]
	col := p.col - v.from
	if v.folded && p.row != v.row {
		col = runeLen(e.segment(v))
	}
	x := e.textX() + measureRunes(e.segment(v), col, e.textSize())
	return x, 4 + float32(vi)*e.lineHeight() - e.scrollY
}

// posToCursor maps a point to the document position nearest to it.
func (e *codeEditor) posToCursor(pos fyne.Position) (row, col int) {
	vls := e.layout()
	vi := int((pos.Y - 4 + e.scrollY) / e.lineHeight())
	if vi < 0 {
		vi = 0
	}
	if vi >= len(vls) {
		vi = len(vls) - 1
	}
	v := vls[vi]
	return v.row, v.from + e.colAtX(e.segment(v), pos.X-e.textX(), v.last)
}

// colAtX is the column nearest x in a segment's text. On a segment that soft
// wrap continues, the column stays on that segment.
func (e *codeEditor) colAtX(text string, x float32, last bool) int {
	if x <= 0 {
		return 0
	}
	size := e.textSize()
	runes := []rune(text)
	width := float32(0)
	for i, r := range runes {
		w := measureRunes(string(r), 1, size)
		if x < width+w/2 {
			return i
		}
		width += w
	}
	if !last && len(runes) > 0 {
		return len(runes) - 1
	}
	return len(runes)
}

// pastLineEnd reports whether x lies beyond the row's last glyph, where the
// only thing a double-click can select is the line itself.
func (e *codeEditor) pastLineEnd(row int, x float32) bool {
	if row < 0 || row >= len(e.lines) {
		return false
	}
	v := e.layout()[e.vlineOf(tpos{row, e.lineLen(row)})]
	end := e.textX() + measureRunes(e.segment(v), runeLen(e.segment(v)), e.textSize())
	return x >= end
}

// ---- scrolling --------------------------------------------------------------

const scrollbarSize = 10

func (e *codeEditor) contentHeight() float32 {
	return float32(len(e.layout()))*e.lineHeight() + 8
}

func (e *codeEditor) maxScrollY(viewH float32) float32 {
	if viewH <= 0 {
		viewH = e.MinSize().Height
	}
	maxY := e.contentHeight() - viewH
	if maxY < 0 {
		return 0
	}
	return maxY
}

// widest is the width of the longest line, for horizontal scrolling.
func (e *codeEditor) widest() float32 {
	size := e.textSize()
	best := float32(0)
	for _, l := range e.lines {
		if w := measureRunes(l, runeLen(l), size); w > best {
			best = w
		}
	}
	return best
}

func (e *codeEditor) maxScrollX() float32 {
	if e.wrap {
		return 0
	}
	maxX := e.widest() + e.gutterWidth() + 8 + e.charWidth()*4 - e.viewWidth()
	if maxX < 0 {
		return 0
	}
	return maxX
}

func (e *codeEditor) clampScroll(viewH float32) {
	if maxY := e.maxScrollY(viewH); e.scrollY > maxY {
		e.scrollY = maxY
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if e.scrollX < 0 || e.wrap {
		e.scrollX = 0
	}
}

// revealCursor scrolls so the primary caret is in view, unfolding a fold the
// caret landed inside.
func (e *codeEditor) revealCursor() {
	e.unfoldAtCarets()
	lh := e.lineHeight()
	viewH := e.viewHeight()
	vi := e.vlineOf(tpos{e.row, e.col})
	top := float32(vi)*lh + 4
	bottom := top + lh
	if top < e.scrollY+4 {
		e.scrollY = top - 4
	}
	if bottom > e.scrollY+viewH-4 {
		e.scrollY = bottom - viewH + 8
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if e.wrap {
		e.scrollX = 0
		return
	}
	v := e.layout()[vi]
	x := e.gutterWidth() + 8 + measureRunes(e.segment(v), e.col-v.from, e.textSize())
	left := e.gutterWidth() + 8
	viewW := e.viewWidth() - scrollbarSize
	if x-e.scrollX < left+e.charWidth()*2 {
		e.scrollX = x - left - e.charWidth()*6
	}
	if x-e.scrollX > viewW-e.charWidth()*3 {
		e.scrollX = x - viewW + e.charWidth()*6
	}
	if e.scrollX < 0 {
		e.scrollX = 0
	}
}

// ensureCursorVisible is revealCursor under its old name.
func (e *codeEditor) ensureCursorVisible() { e.revealCursor() }

// ---- folding --------------------------------------------------------------

// foldEnd is the last row a fold starting at row would hide, if row starts
// a fold at all.
func (e *codeEditor) foldEnd(row int) (int, bool) {
	if e.lang.fold != nil {
		return e.lang.fold(e.lines, row)
	}
	return indentFold(e.lines, row)
}

// indentFold folds a line over the lines below it that are indented deeper;
// blank lines inside go with them, trailing ones do not.
func indentFold(lines []string, row int) (int, bool) {
	if row >= len(lines) || strings.TrimSpace(lines[row]) == "" {
		return 0, false
	}
	base := leadingSpaces(lines[row])
	end := row
	for r := row + 1; r < len(lines); r++ {
		if strings.TrimSpace(lines[r]) == "" {
			continue
		}
		if leadingSpaces(lines[r]) <= base {
			break
		}
		end = r
	}
	return end, end > row
}

// shiftFolds keeps the folds on their lines when an edit replaced rows
// from..to with newRows+1 rows.
func (e *codeEditor) shiftFolds(from, to, newRows int) {
	if len(e.folds) == 0 {
		return
	}
	delta := newRows - (to - from)
	next := make(map[int]bool, len(e.folds))
	for row := range e.folds {
		switch {
		case row <= from:
			next[row] = true
		case row > to:
			next[row+delta] = true
		}
	}
	e.folds = next
	e.foldGen++
}

func (e *codeEditor) toggleFold(row int) {
	if e.folds[row] {
		delete(e.folds, row)
	} else if _, ok := e.foldEnd(row); ok {
		e.folds[row] = true
	} else {
		return
	}
	e.foldGen++
	e.invalidateLayout()
	e.clampScroll(e.viewHeight())
	e.Refresh()
}

// foldAtCaret folds the innermost foldable block around the caret.
func (e *codeEditor) foldAtCaret() {
	for row := e.row; row >= 0; row-- {
		if e.folds[row] {
			continue
		}
		if end, ok := e.foldEnd(row); ok && end >= e.row {
			e.folds[row] = true
			e.foldGen++
			e.setSels([]selRange{caretAt(tpos{row, e.lineLen(row)})}, 0)
			e.invalidateLayout()
			e.revealCursor()
			e.Refresh()
			return
		}
	}
}

// unfoldAtCaret opens the folds on or around the caret's line.
func (e *codeEditor) unfoldAtCaret() {
	changed := false
	for row := range e.folds {
		if end, ok := e.foldEnd(row); ok && row <= e.row && e.row <= end || row == e.row {
			delete(e.folds, row)
			changed = true
		}
	}
	if changed {
		e.foldGen++
		e.invalidateLayout()
		e.Refresh()
	}
}

func (e *codeEditor) foldAll() {
	for row := range e.lines {
		if _, ok := e.foldEnd(row); ok {
			e.folds[row] = true
		}
	}
	e.foldGen++
	e.invalidateLayout()
	e.unfoldAtCarets()
	e.clampScroll(e.viewHeight())
	e.Refresh()
}

func (e *codeEditor) unfoldAll() {
	if len(e.folds) == 0 {
		return
	}
	e.folds = map[int]bool{}
	e.foldGen++
	e.invalidateLayout()
	e.Refresh()
}

// unfoldAtCarets opens any fold that hides a caret.
func (e *codeEditor) unfoldAtCarets() {
	if len(e.folds) == 0 {
		return
	}
	changed := false
	for _, s := range e.allSels() {
		for _, p := range []tpos{s.head, s.anchor} {
			for row := range e.folds {
				if end, ok := e.foldEnd(row); ok && row < p.row && p.row <= end {
					delete(e.folds, row)
					changed = true
				}
			}
		}
	}
	if changed {
		e.foldGen++
		e.invalidateLayout()
	}
}
