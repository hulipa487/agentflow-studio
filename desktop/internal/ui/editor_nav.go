package ui

import (
	"unicode"
)

// moveEach moves every selection's caret with fn; extend keeps the anchors
// (Shift), otherwise each selection collapses to its caret.
func (e *codeEditor) moveEach(extend bool, fn func(s selRange) (tpos, int)) {
	e.touchCaret()
	sels := e.allSels()
	for i, s := range sels {
		head, goal := fn(s)
		head = e.clampPos(head)
		if extend {
			sels[i] = selRange{anchor: s.anchor, head: head, goal: goal}
		} else {
			sels[i] = selRange{anchor: head, head: head, goal: goal}
		}
	}
	e.setSels(sels, 0)
	e.revealCursor()
	e.comp.visible = false
	e.cursorMoved()
}

// move is the arrow keys: dRow lines down (negative is up) along visual lines,
// keeping the column the caret aimed for, or dCol runes along the text,
// crossing line breaks. Without Shift a selection collapses to the side it
// moves towards.
func (e *codeEditor) move(dRow, dCol int, extend bool) {
	e.moveEach(extend, func(s selRange) (tpos, int) {
		if dRow != 0 {
			return e.verticalFrom(s, dRow)
		}
		if !extend && !s.empty() {
			if dCol < 0 {
				return s.from(), -1
			}
			return s.to(), -1
		}
		return e.stepCol(s.head, dCol), -1
	})
}

// verticalFrom moves a caret n visual lines, keeping its goal column.
func (e *codeEditor) verticalFrom(s selRange, n int) (tpos, int) {
	vls := e.layout()
	vi := e.vlineOf(s.head)
	v := vls[vi]
	goal := s.goal
	if goal < 0 {
		goal = s.head.col - v.from
	}
	target := vi + n
	if target < 0 {
		return tpos{0, 0}, goal
	}
	if target >= len(vls) {
		last := len(e.lines) - 1
		return tpos{last, e.lineLen(last)}, goal
	}
	nv := vls[target]
	limit := nv.to
	if !nv.last && limit > nv.from {
		limit--
	}
	col := nv.from + goal
	if col > limit {
		col = limit
	}
	return tpos{nv.row, col}, goal
}

// stepCol moves one rune left or right, onto the neighbouring visible line at
// a line edge.
func (e *codeEditor) stepCol(p tpos, d int) tpos {
	if d < 0 {
		if p.col > 0 {
			return tpos{p.row, p.col - 1}
		}
		row := e.prevVisibleRow(p.row)
		if row < 0 {
			return p
		}
		return tpos{row, e.lineLen(row)}
	}
	if p.col < e.lineLen(p.row) && !e.folds[p.row] {
		return tpos{p.row, p.col + 1}
	}
	if e.folds[p.row] && p.col < e.lineLen(p.row) {
		return tpos{p.row, p.col + 1}
	}
	row := e.nextVisibleRow(p.row)
	if row < 0 {
		return p
	}
	return tpos{row, 0}
}

func (e *codeEditor) prevVisibleRow(row int) int {
	e.layout()
	for r := row - 1; r >= 0; r-- {
		if e.firstV[r] >= 0 {
			return r
		}
	}
	return -1
}

func (e *codeEditor) nextVisibleRow(row int) int {
	e.layout()
	for r := row + 1; r < len(e.lines); r++ {
		if e.firstV[r] >= 0 {
			return r
		}
	}
	return -1
}

// home goes to the first non-blank character, then to the line start.
func (e *codeEditor) home(extend bool) {
	e.moveEach(extend, func(s selRange) (tpos, int) {
		ind := leadingSpaces(e.lines[s.head.row])
		if s.head.col == ind {
			return tpos{s.head.row, 0}, -1
		}
		return tpos{s.head.row, ind}, -1
	})
}

func (e *codeEditor) end(extend bool) {
	e.moveEach(extend, func(s selRange) (tpos, int) {
		return tpos{s.head.row, e.lineLen(s.head.row)}, -1
	})
}

func (e *codeEditor) docEdge(endOfDoc, extend bool) {
	e.moveEach(extend, func(s selRange) (tpos, int) {
		if endOfDoc {
			last := len(e.lines) - 1
			return tpos{last, e.lineLen(last)}, -1
		}
		return tpos{}, -1
	})
	if !extend {
		e.collapseToPrimary()
	}
}

// page moves by a screenful and scrolls with it.
func (e *codeEditor) page(dir int, extend bool) {
	rows := int(e.viewHeight()/e.lineHeight()) - 1
	if rows < 1 {
		rows = 1
	}
	e.scrollY += float32(dir*rows) * e.lineHeight()
	e.clampScroll(e.viewHeight())
	e.move(dir*rows, 0, extend)
}

// wordMove jumps over a word left or right, onto the neighbouring line at an
// edge.
func (e *codeEditor) wordMove(left bool, extend bool) {
	e.moveEach(extend, func(s selRange) (tpos, int) {
		p := s.head
		line := []rune(e.lines[p.row])
		col := p.col
		if left {
			if col == 0 {
				return e.stepCol(p, -1), -1
			}
			for col > 0 && unicode.IsSpace(line[col-1]) {
				col--
			}
			if col > 0 && isWordRune(line[col-1]) {
				for col > 0 && isWordRune(line[col-1]) {
					col--
				}
			} else if col > 0 {
				for col > 0 && !isWordRune(line[col-1]) && !unicode.IsSpace(line[col-1]) {
					col--
				}
			}
			return tpos{p.row, col}, -1
		}
		if col >= len(line) {
			return e.stepCol(p, 1), -1
		}
		if isWordRune(line[col]) {
			for col < len(line) && isWordRune(line[col]) {
				col++
			}
		} else if !unicode.IsSpace(line[col]) {
			for col < len(line) && !isWordRune(line[col]) && !unicode.IsSpace(line[col]) {
				col++
			}
		}
		for col < len(line) && unicode.IsSpace(line[col]) {
			col++
		}
		return tpos{p.row, col}, -1
	})
}

func (e *codeEditor) selectAll() {
	e.touchCaret()
	last := len(e.lines) - 1
	e.setSels([]selRange{{anchor: tpos{}, head: tpos{last, e.lineLen(last)}, goal: -1}}, 0)
	e.comp.visible = false
	e.revealCursor()
	e.Refresh()
}

// selectRange makes one selection from a to b and reveals it.
func (e *codeEditor) selectRange(a, b tpos) {
	e.touchCaret()
	e.setSels([]selRange{{anchor: a, head: b, goal: -1}}, 0)
	e.comp.visible = false
	e.hoverVisible = false
	e.revealCursor()
	e.cursorMoved()
}

// selectLine selects row in full, including its line break when there is a
// following line, so that typing or pasting replaces exactly that line.
func (e *codeEditor) selectLine(row int) {
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	end := tpos{row, e.lineLen(row)}
	if row+1 < len(e.lines) {
		end = tpos{row + 1, 0}
	}
	e.selectRange(tpos{row, 0}, end)
}

// selectWord selects the identifier, whitespace run or single punctuation rune
// at row:col, as a double-click does.
func (e *codeEditor) selectWord(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	line := []rune(e.lines[row])
	if len(line) == 0 {
		e.selectRange(tpos{row, 0}, tpos{row, 0})
		return
	}
	if col < 0 {
		col = 0
	}
	if col >= len(line) {
		col = len(line) - 1
	}
	start, end := col, col+1
	switch {
	case unicode.IsSpace(line[col]):
		for start > 0 && unicode.IsSpace(line[start-1]) {
			start--
		}
		for end < len(line) && unicode.IsSpace(line[end]) {
			end++
		}
	case isWordRune(line[col]):
		for start > 0 && isWordRune(line[start-1]) {
			start--
		}
		for end < len(line) && isWordRune(line[end]) {
			end++
		}
	}
	e.selectRange(tpos{row, start}, tpos{row, end})
}
