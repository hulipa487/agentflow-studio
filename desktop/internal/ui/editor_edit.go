package ui

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// selEdit is what a per-selection command does to one selection: replace the
// runes [from, to) with text, then select a..h, both relative to from in the
// new document.
type selEdit struct {
	from, to int
	text     string
	a, h     int
}

// editEach runs fn for every selection in document order and commits the
// result as one transaction. fn returns ok=false to leave a selection alone.
func (e *codeEditor) editEach(kind string, fn func(s selRange, from, to int) (selEdit, bool)) {
	sels, primary := e.sortedSels()
	changes := make([]change, 0, len(sels))
	out := make([]offSel, 0, len(sels))
	delta, end := 0, -1
	for _, s := range sels {
		from, to := e.offset(s.from()), e.offset(s.to())
		ed, ok := fn(s, from, to)
		if !ok || ed.from < end {
			out = append(out, offSel{e.offset(s.anchor) + delta, e.offset(s.head) + delta, s.goal})
			continue
		}
		changes = append(changes, change{from: ed.from, to: ed.to, text: ed.text})
		base := ed.from + delta
		out = append(out, offSel{base + ed.a, base + ed.h, -1})
		delta += runeLen(ed.text) - (ed.to - ed.from)
		end = ed.to
	}
	e.commit(changes, out, primary, kind)
}

// resetAfter replaces the selections the last commit left, and the history's
// record of them, for commands that place carets by row rather than offset.
func (e *codeEditor) resetAfter(sels []selRange, primary int) {
	e.setSels(sels, primary)
	if n := len(e.undos); n > 0 {
		g := &e.undos[n-1]
		g.entries[len(g.entries)-1].after = e.allSels()
	}
	e.revealCursor()
	e.Refresh()
}

func (e *codeEditor) runeAt(row, col int) rune {
	if row < 0 || row >= len(e.lines) || col < 0 {
		return 0
	}
	r := []rune(e.lines[row])
	if col >= len(r) {
		return 0
	}
	return r[col]
}

// ---- typing ---------------------------------------------------------------

// insert types r at every selection: brackets and quotes pair, a selection is
// wrapped by an opening bracket or quote, a typed closer steps over the one
// already there, and a closing keyword re-indents its line.
func (e *codeEditor) insert(r rune) {
	if r == '\n' {
		e.newline()
		return
	}
	pairs := e.lang.pairs
	e.editEach("type", func(s selRange, from, to int) (selEdit, bool) {
		ch := string(r)
		closer, opens := pairs[r]
		if !s.empty() {
			if opens {
				inner := e.textRange(from, to)
				return selEdit{from, to, ch + inner + string(closer), 1, 1 + runeLen(inner)}, true
			}
			return selEdit{from, to, ch, 1, 1}, true
		}
		row, col := s.head.row, s.head.col
		next, prev := e.runeAt(row, col), e.runeAt(row, col-1)
		if isCloser(pairs, r) && next == r {
			return selEdit{from, from, "", 1, 1}, true
		}
		if opens {
			if closer == r { // a quote
				if prev != 0 && (isWordRune(prev) || prev == r) {
					return selEdit{from, from, ch, 1, 1}, true
				}
			}
			if next == 0 || unicode.IsSpace(next) || strings.ContainsRune(")]},;:", next) {
				return selEdit{from, from, ch + string(closer), 1, 1}, true
			}
		}
		return selEdit{from, from, ch, 1, 1}, true
	})
	e.electricIndent()
	if e.onAutoComplete != nil && (isWordRune(r) || r == '.' || r == ':') {
		e.onAutoComplete()
	}
}

func isCloser(pairs map[rune]rune, r rune) bool {
	for _, c := range pairs {
		if c == r {
			return true
		}
	}
	return false
}

// electricIndent re-indents a line whose text became a block closer (end,
// else, until, a closing bracket) so it lines up with its opener.
func (e *codeEditor) electricIndent() {
	if e.lang.closes == nil {
		return
	}
	unit := e.lang.unit
	var changes []change
	seen := map[int]bool{}
	for _, s := range e.allSels() {
		row := s.head.row
		if seen[row] || !s.empty() {
			continue
		}
		seen[row] = true
		trimmed := strings.TrimSpace(e.lines[row])
		if !e.lang.closes(trimmed) || s.head.col != e.lineLen(row) {
			continue
		}
		prev := row - 1
		for prev >= 0 && strings.TrimSpace(e.lines[prev]) == "" {
			prev--
		}
		if prev < 0 {
			continue
		}
		want := leadingSpaces(e.lines[prev])
		if !e.lang.opens(strings.TrimSpace(e.lines[prev])) {
			want -= unit
		}
		have := leadingSpaces(e.lines[row])
		if want < 0 || want == have {
			continue
		}
		start := e.starts()[row]
		changes = append(changes, change{from: start, to: start + have, text: strings.Repeat(" ", want)})
	}
	if len(changes) > 0 {
		e.commit(changes, nil, 0, "type")
		for i, s := range e.extra {
			e.extra[i] = caretAt(tpos{s.head.row, e.lineLen(s.head.row)})
		}
		e.col = e.lineLen(e.row)
		e.Refresh()
	}
}

var listMarker = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+)(\[[ xX]\]\s+)?`)

// newline breaks the line at every selection and indents the new line: as
// deep as the old one, a level deeper after a block opener, and with the
// closer on its own line when the caret sat between a bracket pair. Markdown
// lists continue; an empty item ends the list.
func (e *codeEditor) newline() {
	unit := strings.Repeat(" ", e.lang.unit)
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		row := s.from().row
		line := []rune(e.lines[row])
		col := minInt(s.from().col, len(line))
		before := string(line[:col])
		lead := strings.Repeat(" ", leadingSpaces(e.lines[row]))
		if e.lang.name == "markdown" {
			if m := listMarker.FindStringSubmatch(before); m != nil {
				if strings.TrimSpace(before) == strings.TrimSpace(m[0]) && s.empty() && col == len(line) {
					// An empty item: drop the marker instead of continuing.
					start := e.starts()[row]
					return selEdit{start, start + len(line), "", 0, 0}, true
				}
				marker := m[2]
				if n, err := strconv.Atoi(strings.TrimRight(marker, ".)")); err == nil {
					marker = strconv.Itoa(n+1) + marker[len(marker)-1:]
				}
				check := ""
				if m[4] != "" {
					check = "[ ] "
				}
				text := "\n" + m[1] + marker + m[3] + check
				return selEdit{from, to, text, runeLen(text), runeLen(text)}, true
			}
		}
		indent := lead
		if e.lang.opens(strings.TrimSpace(before)) {
			indent += unit
		}
		prev, next := e.runeAt(row, col-1), e.runeAt(s.to().row, s.to().col)
		if closer, ok := e.lang.pairs[prev]; ok && closer == next && prev != next {
			text := "\n" + lead + unit + "\n" + lead
			caret := 1 + runeLen(lead+unit)
			return selEdit{from, to, text, caret, caret}, true
		}
		if next == ')' {
			// Keep a closing call on its own indentation level.
			indent = lead
		}
		text := "\n" + indent
		return selEdit{from, to, text, runeLen(text), runeLen(text)}, true
	})
}

func leadingSpaces(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' {
			break
		}
		n++
	}
	return n
}

// ---- deleting ---------------------------------------------------------------

func (e *codeEditor) backspace() {
	unit := e.lang.unit
	pairs := e.lang.pairs
	e.editEach("delete", func(s selRange, from, to int) (selEdit, bool) {
		if !s.empty() {
			return selEdit{from, to, "", 0, 0}, true
		}
		row, col := s.head.row, s.head.col
		if col == 0 {
			if row == 0 {
				return selEdit{}, false
			}
			return selEdit{from - 1, from, "", 0, 0}, true
		}
		prev, next := e.runeAt(row, col-1), e.runeAt(row, col)
		if closer, ok := pairs[prev]; ok && closer == next {
			return selEdit{from - 1, from + 1, "", 0, 0}, true
		}
		if col <= leadingSpaces(e.lines[row]) && unit > 1 {
			// In the indentation, step back to the previous indent stop.
			n := col - (col-1)/unit*unit
			return selEdit{from - n, from, "", 0, 0}, true
		}
		return selEdit{from - 1, from, "", 0, 0}, true
	})
}

func (e *codeEditor) deleteForward() {
	e.editEach("delete", func(s selRange, from, to int) (selEdit, bool) {
		if !s.empty() {
			return selEdit{from, to, "", 0, 0}, true
		}
		if from >= e.docLen() {
			return selEdit{}, false
		}
		return selEdit{from, from + 1, "", 0, 0}, true
	})
}

func (e *codeEditor) deleteWordLeft() {
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		if !s.empty() {
			return selEdit{from, to, "", 0, 0}, true
		}
		row, col := s.head.row, s.head.col
		if col == 0 {
			if row == 0 {
				return selEdit{}, false
			}
			return selEdit{from - 1, from, "", 0, 0}, true
		}
		line := []rune(e.lines[row])
		start := col
		for start > 0 && unicode.IsSpace(line[start-1]) {
			start--
		}
		for start > 0 && isWordRune(line[start-1]) {
			start--
		}
		if start == col && start > 0 {
			start--
		}
		return selEdit{from - (col - start), from, "", 0, 0}, true
	})
}

func (e *codeEditor) deleteWordRight() {
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		if !s.empty() {
			return selEdit{from, to, "", 0, 0}, true
		}
		row, col := s.head.row, s.head.col
		line := []rune(e.lines[row])
		if col >= len(line) {
			if row >= len(e.lines)-1 {
				return selEdit{}, false
			}
			return selEdit{from, from + 1, "", 0, 0}, true
		}
		end := col
		for end < len(line) && unicode.IsSpace(line[end]) {
			end++
		}
		for end < len(line) && isWordRune(line[end]) {
			end++
		}
		for end < len(line) && unicode.IsSpace(line[end]) {
			end++
		}
		if end == col {
			end++
		}
		return selEdit{from, from + (end - col), "", 0, 0}, true
	})
}

// ---- line commands ----------------------------------------------------------

// lineBlocks returns the line ranges the selections cover, merged and in
// order. A selection ending at the start of a line does not take that line.
func (e *codeEditor) lineBlocks() [][2]int {
	var blocks [][2]int
	sels, _ := e.sortedSels()
	for _, s := range sels {
		r1, r2 := s.from().row, s.to().row
		if r2 > r1 && s.to().col == 0 {
			r2--
		}
		if n := len(blocks); n > 0 && r1 <= blocks[n-1][1]+1 {
			if r2 > blocks[n-1][1] {
				blocks[n-1][1] = r2
			}
			continue
		}
		blocks = append(blocks, [2]int{r1, r2})
	}
	return blocks
}

func (e *codeEditor) lineStart(row int) int { return e.starts()[row] }
func (e *codeEditor) lineEnd(row int) int   { return e.starts()[row] + e.lineLen(row) }

// indent is Tab and Shift+Tab: carets get spaces to the next indent stop;
// selections (and Shift+Tab) shift whole lines by one level.
func (e *codeEditor) indent(outdent bool) {
	unit := e.lang.unit
	if !outdent {
		allCarets := true
		for _, s := range e.allSels() {
			if !s.empty() {
				allCarets = false
				break
			}
		}
		if allCarets {
			e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
				n := unit - s.head.col%unit
				return selEdit{from, from, strings.Repeat(" ", n), n, n}, true
			})
			return
		}
	}
	e.shiftLines(outdent)
}

func (e *codeEditor) shiftLines(outdent bool) {
	unit := e.lang.unit
	var changes []change
	for _, b := range e.lineBlocks() {
		for row := b[0]; row <= b[1]; row++ {
			start := e.lineStart(row)
			if outdent {
				n := minInt(leadingSpaces(e.lines[row]), unit)
				if n > 0 {
					changes = append(changes, change{from: start, to: start + n})
				}
			} else if strings.TrimSpace(e.lines[row]) != "" || b[0] == b[1] {
				changes = append(changes, change{from: start, to: start, text: strings.Repeat(" ", unit)})
			}
		}
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
}

// toggleComment comments or uncomments the selected lines with the language's
// line comment, or wraps them in its block comment when it has none.
func (e *codeEditor) toggleComment() {
	tok := e.lang.lineComment
	if tok == "" {
		e.toggleBlockComment()
		return
	}
	var changes []change
	for _, b := range e.lineBlocks() {
		commented, minIndent := true, -1
		for row := b[0]; row <= b[1]; row++ {
			t := strings.TrimSpace(e.lines[row])
			if t == "" {
				continue
			}
			if !strings.HasPrefix(t, tok) {
				commented = false
			}
			if ind := leadingSpaces(e.lines[row]); minIndent < 0 || ind < minIndent {
				minIndent = ind
			}
		}
		if minIndent < 0 {
			continue // only blank lines
		}
		for row := b[0]; row <= b[1]; row++ {
			line := e.lines[row]
			if strings.TrimSpace(line) == "" {
				continue
			}
			start := e.lineStart(row)
			if commented {
				ind := leadingSpaces(line)
				n := runeLen(tok)
				if strings.HasPrefix(line[ind+len(tok):], " ") {
					n++
				}
				changes = append(changes, change{from: start + ind, to: start + ind + n})
			} else {
				changes = append(changes, change{from: start + minIndent, to: start + minIndent, text: tok + " "})
			}
		}
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
}

func (e *codeEditor) toggleBlockComment() {
	open, close := e.lang.blockComment[0], e.lang.blockComment[1]
	if open == "" {
		return
	}
	var changes []change
	for _, b := range e.lineBlocks() {
		first, last := e.lines[b[0]], e.lines[b[1]]
		ft, lt := strings.TrimSpace(first), strings.TrimSpace(last)
		if strings.HasPrefix(ft, open) && strings.HasSuffix(lt, close) {
			fi := e.lineStart(b[0]) + runeLen(first[:strings.Index(first, open)])
			n := runeLen(open)
			if strings.HasPrefix(first[strings.Index(first, open)+len(open):], " ") {
				n++
			}
			li := strings.LastIndex(last, close)
			le := e.lineStart(b[1]) + runeLen(last[:li])
			m := runeLen(close)
			if li > 0 && last[li-1] == ' ' {
				le--
				m++
			}
			changes = append(changes, change{from: fi, to: fi + n}, change{from: le, to: le + m})
			continue
		}
		ind := leadingSpaces(first)
		changes = append(changes,
			change{from: e.lineStart(b[0]) + ind, to: e.lineStart(b[0]) + ind, text: open + " "},
			change{from: e.lineEnd(b[1]), to: e.lineEnd(b[1]), text: " " + close})
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
}

// copyLines duplicates the selected lines below (down) or above them; the
// selections follow the copy in the direction given.
func (e *codeEditor) copyLines(down bool) {
	var changes []change
	for _, b := range e.lineBlocks() {
		block := strings.Join(e.lines[b[0]:b[1]+1], "\n")
		if down {
			start := e.lineStart(b[0])
			changes = append(changes, change{from: start, to: start, text: block + "\n"})
		} else {
			end := e.lineEnd(b[1])
			changes = append(changes, change{from: end, to: end, text: "\n" + block})
		}
	}
	e.breakUndo()
	if down {
		e.commit(changes, nil, 0, "")
		return
	}
	// Copying up keeps the carets where they are, on the upper copy.
	sels := e.allSels()
	e.commit(changes, nil, 0, "")
	e.resetAfter(sels, 0)
}

// deleteLines removes the selected lines.
func (e *codeEditor) deleteLines() {
	var changes []change
	var carets []selRange
	removed := 0
	for _, b := range e.lineBlocks() {
		from, to := e.lineStart(b[0]), 0
		if b[1]+1 < len(e.lines) {
			to = e.lineStart(b[1] + 1)
		} else {
			to = e.lineEnd(b[1])
			if b[0] > 0 {
				from = e.lineEnd(b[0] - 1)
			}
		}
		changes = append(changes, change{from: from, to: to})
		row := b[0] - removed
		if b[1]+1 >= len(e.lines) && b[0] > 0 {
			row--
		}
		carets = append(carets, caretAt(tpos{row, e.col}))
		removed += b[1] - b[0] + 1
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
	e.resetAfter(carets, 0)
}

// moveLines moves the selected lines one line up or down.
func (e *codeEditor) moveLines(dir int) {
	blocks := e.lineBlocks()
	if len(blocks) == 0 {
		return
	}
	if dir < 0 && blocks[0][0] == 0 || dir > 0 && blocks[len(blocks)-1][1] == len(e.lines)-1 {
		return
	}
	var changes []change
	for _, b := range blocks {
		block := strings.Join(e.lines[b[0]:b[1]+1], "\n")
		if dir < 0 {
			changes = append(changes, change{from: e.lineStart(b[0] - 1), to: e.lineEnd(b[1]), text: block + "\n" + e.lines[b[0]-1]})
		} else {
			changes = append(changes, change{from: e.lineStart(b[0]), to: e.lineEnd(b[1] + 1), text: e.lines[b[1]+1] + "\n" + block})
		}
	}
	sels := e.allSels()
	for i := range sels {
		sels[i].anchor.row += dir
		sels[i].head.row += dir
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
	e.resetAfter(sels, 0)
}

// joinLines joins each selection's lines (or the caret's line and the next)
// with a single space.
func (e *codeEditor) joinLines() {
	var changes []change
	var carets []int
	for _, b := range e.lineBlocks() {
		last := b[1]
		if last == b[0] {
			last++
		}
		if last >= len(e.lines) {
			continue
		}
		for row := b[0]; row < last; row++ {
			end := e.lineEnd(row)
			next := e.lines[row+1]
			sep := " "
			if strings.TrimRight(e.lines[row], " ") != e.lines[row] || strings.TrimSpace(next) == "" || e.lines[row] == "" {
				sep = ""
			}
			changes = append(changes, change{from: end, to: e.lineStart(row+1) + leadingSpaces(next), text: sep})
			if row == last-1 {
				carets = append(carets, end)
			}
		}
	}
	if len(changes) == 0 {
		return
	}
	sels := make([]offSel, len(carets))
	for i, c := range carets {
		sels[i] = caretOff(mapPos(changes, c, -1))
	}
	e.breakUndo()
	e.commit(changes, sels, 0, "")
}

// changeCase upper- or lower-cases each selection, or the word at a caret.
func (e *codeEditor) changeCase(upper bool) {
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		if s.empty() {
			ws, we := e.wordAt(s.head.row, s.head.col)
			if ws == we {
				return selEdit{}, false
			}
			start := e.lineStart(s.head.row)
			from, to = start+ws, start+we
		}
		text := e.textRange(from, to)
		if upper {
			text = strings.ToUpper(text)
		} else {
			text = strings.ToLower(text)
		}
		a, h := 0, runeLen(text)
		if !s.empty() && s.head.before(s.anchor) {
			a, h = h, a
		}
		if s.empty() {
			a = e.offset(s.head) - from
			h = a
		}
		return selEdit{from, to, text, a, h}, true
	})
}

// wordAt returns the identifier around col on row, as [start, end).
func (e *codeEditor) wordAt(row, col int) (int, int) {
	line := []rune(e.lines[row])
	start, end := col, col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	return start, end
}

func isWordRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }

func isIdentRune(r rune) bool { return isWordRune(r) }

// ---- clipboard ------------------------------------------------------------

// lineClip remembers that the clipboard holds whole lines copied from a bare
// caret, so pasting it at a caret inserts above the line, as in VS Code.
var lineClip string

func (e *codeEditor) copySelection() {
	if t := e.clipText(); t != "" {
		e.clipboard().SetContent(t)
	}
}

// clipText is what copying takes: every selection joined by line breaks, or
// the caret lines whole when nothing is selected.
func (e *codeEditor) clipText() string {
	sels, _ := e.sortedSels()
	anySel := false
	for _, s := range sels {
		if !s.empty() {
			anySel = true
			break
		}
	}
	if !anySel {
		var rows []string
		seen := map[int]bool{}
		for _, s := range sels {
			if !seen[s.head.row] {
				seen[s.head.row] = true
				rows = append(rows, e.lines[s.head.row])
			}
		}
		lineClip = strings.Join(rows, "\n") + "\n"
		return lineClip
	}
	parts := make([]string, 0, len(sels))
	for _, s := range sels {
		parts = append(parts, e.textRange(e.offset(s.from()), e.offset(s.to())))
	}
	lineClip = ""
	return strings.Join(parts, "\n")
}

func (e *codeEditor) cutSelection() {
	t := e.clipText()
	if t == "" {
		return
	}
	e.clipboard().SetContent(t)
	if lineClip != "" {
		e.deleteLines()
		return
	}
	e.breakUndo()
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		return selEdit{from, to, "", 0, 0}, !s.empty()
	})
}

func (e *codeEditor) paste() {
	text := e.clipboard().Content()
	if text == "" {
		return
	}
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\t", "  ")
	e.replaceSelection(text)
}

// replaceSelection inserts text at every selection. Pasting as many lines as
// there are carets puts one line at each; whole lines copied from a caret go
// above the caret's line.
func (e *codeEditor) replaceSelection(text string) {
	e.breakUndo()
	sels, _ := e.sortedSels()
	parts := strings.Split(text, "\n")
	distribute := len(sels) > 1 && len(parts) == len(sels)
	whole := text == lineClip && lineClip != ""
	i := -1
	e.editEach("", func(s selRange, from, to int) (selEdit, bool) {
		i++
		t := text
		if distribute {
			t = parts[i]
		}
		if whole && s.empty() {
			start := e.lineStart(s.head.row)
			n := runeLen(t)
			return selEdit{start, start, t, n + s.head.col, n + s.head.col}, true
		}
		n := runeLen(t)
		return selEdit{from, to, t, n, n}, true
	})
}

// ---- multiple selections --------------------------------------------------

// addNextOccurrence is Ctrl+D: a bare caret selects its word; after that each
// press adds the next occurrence of the selected text.
func (e *codeEditor) addNextOccurrence() {
	p := e.primary()
	if p.empty() {
		ws, we := e.wordAt(p.head.row, p.head.col)
		if ws == we {
			return
		}
		sels := e.allSels()
		sels[0] = selRange{anchor: tpos{p.head.row, ws}, head: tpos{p.head.row, we}, goal: -1}
		e.setSels(sels, 0)
		e.revealCursor()
		e.cursorMoved()
		return
	}
	needle := e.selectionText()
	text := []rune(e.Text())
	nr := []rune(needle)
	taken := map[int]bool{}
	last := 0
	for _, s := range e.allSels() {
		f := e.offset(s.from())
		taken[f] = true
		if f > last {
			last = f
		}
	}
	for pass := 0; pass < 2; pass++ {
		start := last + 1
		if pass == 1 {
			start = 0
		}
		for i := start; i+len(nr) <= len(text); i++ {
			if taken[i] || !runesAt(text, nr, i) {
				continue
			}
			sels := append(e.allSels(), selRange{anchor: e.posAt(i), head: e.posAt(i + len(nr)), goal: -1})
			e.setSels(sels, len(sels)-1)
			e.revealCursor()
			e.cursorMoved()
			return
		}
	}
}

// selectAllOccurrences is Ctrl+Shift+L: a selection on every occurrence of
// the selected text (or the caret's word).
func (e *codeEditor) selectAllOccurrences() {
	if e.primary().empty() {
		e.addNextOccurrence()
		if e.primary().empty() {
			return
		}
	}
	nr := []rune(e.selectionText())
	text := []rune(e.Text())
	pf := e.offset(e.primary().from())
	var sels []selRange
	primary := 0
	for i := 0; i+len(nr) <= len(text); i++ {
		if runesAt(text, nr, i) {
			if i == pf {
				primary = len(sels)
			}
			sels = append(sels, selRange{anchor: e.posAt(i), head: e.posAt(i + len(nr)), goal: -1})
			i += len(nr) - 1
		}
	}
	if len(sels) > 0 {
		e.setSels(sels, primary)
		e.cursorMoved()
	}
}

func runesAt(text, needle []rune, i int) bool {
	for j := range needle {
		if text[i+j] != needle[j] {
			return false
		}
	}
	return true
}

// addCaretVertical is Ctrl+Alt+Up/Down: a caret on the line above or below
// the outermost caret, at the same visual column.
func (e *codeEditor) addCaretVertical(dir int) {
	sels := e.allSels()
	sort.SliceStable(sels, func(i, j int) bool { return sels[i].head.before(sels[j].head) })
	edge := sels[0]
	if dir > 0 {
		edge = sels[len(sels)-1]
	}
	row := edge.head.row + dir
	if row < 0 || row >= len(e.lines) {
		return
	}
	goal := edge.goal
	if goal < 0 {
		goal = edge.head.col
	}
	n := selRange{anchor: tpos{row, goal}, head: tpos{row, goal}, goal: goal}
	all := append(e.allSels(), n)
	e.setSels(all, len(all)-1)
	e.goal = goal
	e.revealCursor()
	e.cursorMoved()
}

// columnSelect selects the rectangle between two positions: one selection
// per line, as Alt+drag does.
func (e *codeEditor) columnSelect(a, b tpos) {
	r1, r2 := a.row, b.row
	if r1 > r2 {
		r1, r2 = r2, r1
	}
	var sels []selRange
	primary := 0
	for row := r1; row <= r2; row++ {
		l := e.lineLen(row)
		sels = append(sels, selRange{anchor: tpos{row, minInt(a.col, l)}, head: tpos{row, minInt(b.col, l)}, goal: b.col})
		if row == b.row {
			primary = len(sels) - 1
		}
	}
	e.setSels(sels, primary)
}

// expandLineSelection is Ctrl+L: every selection grows to whole lines, and a
// second press takes the next line too.
func (e *codeEditor) expandLineSelection() {
	sels := e.allSels()
	for i, s := range sels {
		f, t := s.from(), s.to()
		end := tpos{t.row + 1, 0}
		if t.col == 0 && t.row > f.row && f.col == 0 {
			end = tpos{t.row + 1, 0}
		}
		if end.row >= len(e.lines) {
			end = tpos{len(e.lines) - 1, e.lineLen(len(e.lines) - 1)}
		}
		sels[i] = selRange{anchor: tpos{f.row, 0}, head: end, goal: -1}
	}
	e.setSels(sels, 0)
	e.revealCursor()
	e.cursorMoved()
}

// ---- brackets -------------------------------------------------------------

var bracketPairs = map[rune]rune{'(': ')', '[': ']', '{': '}'}

// matchingBracket finds the bracket that pairs with the one at or just before
// p, skipping strings and comments as the highlighter sees them.
func (e *codeEditor) matchingBracket(p tpos) (at, match tpos, ok bool) {
	for _, col := range []int{p.col, p.col - 1} {
		r := e.runeAt(p.row, col)
		if r == 0 {
			continue
		}
		if closer, open := bracketPairs[r]; open {
			if m, found := e.scanBracket(tpos{p.row, col}, r, closer, 1); found {
				return tpos{p.row, col}, m, true
			}
			return tpos{p.row, col}, tpos{}, false
		}
		for o, c := range bracketPairs {
			if c == r {
				if m, found := e.scanBracket(tpos{p.row, col}, r, o, -1); found {
					return tpos{p.row, col}, m, true
				}
				return tpos{p.row, col}, tpos{}, false
			}
		}
	}
	return tpos{}, tpos{}, false
}

func (e *codeEditor) scanBracket(from tpos, self, other rune, dir int) (tpos, bool) {
	depth := 0
	row, col := from.row, from.col
	for steps := 0; steps < 20000; steps++ {
		r := []rune(e.lines[row])
		if col >= 0 && col < len(r) && !e.inStringOrComment(row, col) {
			switch r[col] {
			case self:
				depth++
			case other:
				depth--
				if depth == 0 {
					return tpos{row, col}, true
				}
			}
		}
		col += dir
		for col < 0 || col >= len([]rune(e.lines[row])) {
			row += dir
			if row < 0 || row >= len(e.lines) {
				return tpos{}, false
			}
			if dir > 0 {
				col = 0
			} else {
				col = len([]rune(e.lines[row])) - 1
			}
			if len([]rune(e.lines[row])) > 0 {
				break
			}
		}
	}
	return tpos{}, false
}

// jumpToBracket is Ctrl+Shift+\: move the caret to the matching bracket.
func (e *codeEditor) jumpToBracket() {
	if _, m, ok := e.matchingBracket(tpos{e.row, e.col}); ok {
		e.SetCursor(m.row, m.col)
	}
}
