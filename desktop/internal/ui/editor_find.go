package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// searchQuery is what the find bar looks for.
type searchQuery struct {
	text                     string
	caseSensitive, wholeWord bool
	regex                    bool
}

const maxMatches = 10000

func (q searchQuery) compile() (*regexp.Regexp, error) {
	pattern := q.text
	if !q.regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	flags := "(?m)"
	if !q.caseSensitive {
		flags = "(?mi)"
	}
	return regexp.Compile(flags + pattern)
}

// setSearch highlights every match of q; an empty query clears them.
func (e *codeEditor) setSearch(q searchQuery) {
	e.search = q
	e.refreshMatches()
	e.Refresh()
}

func (e *codeEditor) clearSearch() { e.setSearch(searchQuery{}) }

// refreshMatches recomputes the matches as rune offsets.
func (e *codeEditor) refreshMatches() {
	e.matches, e.searchErr = nil, nil
	if e.search.text == "" {
		return
	}
	re, err := e.search.compile()
	if err != nil {
		e.searchErr = err
		return
	}
	text := e.Text()
	runeOff, byteOff := 0, 0
	for _, m := range re.FindAllStringIndex(text, maxMatches) {
		if m[1] == m[0] {
			continue // an empty match selects nothing
		}
		runeOff += utf8.RuneCountInString(text[byteOff:m[0]])
		from := runeOff
		runeOff += utf8.RuneCountInString(text[m[0]:m[1]])
		byteOff = m[1]
		if e.search.wholeWord {
			before, _ := utf8.DecodeLastRuneInString(text[:m[0]])
			after, _ := utf8.DecodeRuneInString(text[m[1]:])
			if (m[0] > 0 && isWordRune(before)) || (m[1] < len(text) && isWordRune(after)) {
				continue
			}
		}
		e.matches = append(e.matches, change{from: from, to: runeOff})
	}
}

// currentMatch is the index of the match the primary selection covers, or -1.
func (e *codeEditor) currentMatch() int {
	p := e.primary()
	f, t := e.offset(p.from()), e.offset(p.to())
	for i, m := range e.matches {
		if m.from == f && m.to == t {
			return i
		}
	}
	return -1
}

// findNext selects the next (or previous) match, wrapping at the ends. With
// no search yet, F3 searches for the selected text.
func (e *codeEditor) findNext(backward bool) {
	if e.search.text == "" {
		t := e.selectionText()
		if t == "" || strings.Contains(t, "\n") {
			return
		}
		e.setSearch(searchQuery{text: t, caseSensitive: true})
	}
	if len(e.matches) == 0 {
		return
	}
	p := e.primary()
	var m change
	if backward {
		at := e.offset(p.from())
		m = e.matches[len(e.matches)-1]
		for i := len(e.matches) - 1; i >= 0; i-- {
			if e.matches[i].from < at {
				m = e.matches[i]
				break
			}
		}
	} else {
		at := e.offset(p.to())
		if p.empty() {
			at = e.offset(p.head)
		}
		m = e.matches[0]
		for _, c := range e.matches {
			if c.from >= at && !(c.from == e.offset(p.from()) && c.to == at && !p.empty()) {
				m = c
				break
			}
		}
	}
	e.selectRange(e.posAt(m.from), e.posAt(m.to))
}

// findFrom selects the first match at or after off, for incremental search.
func (e *codeEditor) findFrom(off int) {
	if len(e.matches) == 0 {
		return
	}
	m := e.matches[0]
	for _, c := range e.matches {
		if c.from >= off {
			m = c
			break
		}
	}
	e.selectRange(e.posAt(m.from), e.posAt(m.to))
}

// replacement expands repl for one match ($1 and friends with regex).
func (e *codeEditor) replacement(matched, repl string) string {
	if !e.search.regex {
		return repl
	}
	re, err := e.search.compile()
	if err != nil {
		return repl
	}
	return re.ReplaceAllString(matched, repl)
}

// replaceCurrent replaces the selected match and moves to the next one.
func (e *codeEditor) replaceCurrent(repl string) {
	i := e.currentMatch()
	if i < 0 {
		e.findNext(false)
		return
	}
	m := e.matches[i]
	text := e.replacement(e.textRange(m.from, m.to), repl)
	e.breakUndo()
	end := m.from + runeLen(text)
	e.commit([]change{{from: m.from, to: m.to, text: text}}, []offSel{caretOff(end)}, 0, "")
	e.findFrom(end)
}

// replaceAll replaces every match in one undo step and reports how many.
func (e *codeEditor) replaceAll(repl string) int {
	if len(e.matches) == 0 {
		return 0
	}
	changes := make([]change, len(e.matches))
	for i, m := range e.matches {
		changes[i] = change{from: m.from, to: m.to, text: e.replacement(e.textRange(m.from, m.to), repl)}
	}
	e.breakUndo()
	e.commit(changes, nil, 0, "")
	return len(changes)
}

// selectAllMatches puts a selection on every match.
func (e *codeEditor) selectAllMatches() {
	if len(e.matches) == 0 {
		return
	}
	sels := make([]selRange, len(e.matches))
	for i, m := range e.matches {
		sels[i] = selRange{anchor: e.posAt(m.from), head: e.posAt(m.to), goal: -1}
	}
	cur := maxInt(e.currentMatch(), 0)
	e.setSels(sels, cur)
	e.revealCursor()
	e.cursorMoved()
}

// ---- the bar ------------------------------------------------------------------

// barEntry is an Entry that hands Escape and Shift+Enter to the bar instead
// of the window, so closing the bar never leaves the screen.
type barEntry struct {
	widget.Entry
	onEsc, onShiftEnter, onAltEnter func()
}

func newBarEntry(placeholder string) *barEntry {
	b := &barEntry{}
	b.ExtendBaseWidget(b)
	b.PlaceHolder = placeholder
	return b
}

func (b *barEntry) TypedKey(k *fyne.KeyEvent) {
	if k.Name == fyne.KeyEscape {
		if b.onEsc != nil {
			b.onEsc()
		}
		return
	}
	b.Entry.TypedKey(k)
}

func (b *barEntry) TypedShortcut(s fyne.Shortcut) {
	if cs, ok := s.(*desktop.CustomShortcut); ok && cs.Modifier == fyne.KeyModifierAlt &&
		(cs.KeyName == fyne.KeyReturn || cs.KeyName == fyne.KeyEnter) {
		if b.onAltEnter != nil {
			b.onAltEnter()
		}
		return
	}
	if cs, ok := s.(*desktop.CustomShortcut); ok && cs.Modifier == fyne.KeyModifierShift &&
		(cs.KeyName == fyne.KeyReturn || cs.KeyName == fyne.KeyEnter) {
		if b.onShiftEnter != nil {
			b.onShiftEnter()
		}
		return
	}
	b.Entry.TypedShortcut(s)
}

// codeArea is the editor with its find/replace and go-to-line bar above it
// and a status line below: the frame every code document in Studio uses.
type codeArea struct {
	editor *codeEditor
	root   fyne.CanvasObject

	barSlot   *fyne.Container
	findRow   fyne.CanvasObject
	replRow   fyne.CanvasObject
	gotoRow   fyne.CanvasObject
	find      *barEntry
	repl      *barEntry
	gotoLine  *barEntry
	count     *widget.Label
	caseBtn   *toggleButton
	wordBtn   *toggleButton
	regexBtn  *toggleButton
	status    *widget.Label
	startOff  int
	replacing bool
}

// newCodeAreaText builds an editor for source in lang, framed.
func newCodeAreaText(source, lang string) *codeArea {
	return newCodeArea(newCodeEditorLang(source, lang))
}

func newCodeArea(ed *codeEditor) *codeArea {
	a := &codeArea{editor: ed}
	ed.onFind = a.openFind
	ed.onGoto = a.openGoto
	prev := ed.onCursor
	ed.onCursor = func() {
		if prev != nil {
			prev()
		}
		a.updateStatus()
	}

	a.find = newBarEntry(T("editor.find.placeholder"))
	a.find.OnChanged = func(string) { a.runSearch(true) }
	a.find.OnSubmitted = func(string) { a.next(false) }
	a.find.onShiftEnter = func() { a.next(true) }
	a.find.onEsc = a.close
	a.find.onAltEnter = func() {
		a.editor.selectAllMatches()
		a.focusEditor()
	}
	a.repl = newBarEntry(T("editor.find.replacePlaceholder"))
	a.repl.OnSubmitted = func(string) { a.replaceOne() }
	a.repl.onEsc = a.close
	a.gotoLine = newBarEntry("")
	a.gotoLine.OnSubmitted = func(string) { a.jump() }
	a.gotoLine.onEsc = a.close

	a.count = widget.NewLabel("")
	a.count.SizeName = theme.SizeNameCaptionText
	a.count.Importance = widget.LowImportance
	opt := func(label, tip string) *toggleButton {
		var t *toggleButton
		t = newToggleButton(label, nil, func() {
			t.setActive(!t.active)
			a.runSearch(true)
		})
		return t
	}
	a.caseBtn = opt("Aa", T("editor.find.matchCase"))
	a.wordBtn = opt("ab", T("editor.find.wholeWord"))
	a.regexBtn = opt(".*", T("editor.find.regex"))

	small := func(w float32, o fyne.CanvasObject) fyne.CanvasObject { return widthBox(w, o) }
	prevBtn := iconButton(theme.MoveUpIcon(), T("editor.find.previous"), func() { a.next(true) })
	nextBtn := iconButton(theme.MoveDownIcon(), T("editor.find.next"), func() { a.next(false) })
	allBtn := iconButton(theme.ListIcon(), T("editor.find.selectAll"), func() {
		a.editor.selectAllMatches()
		a.focusEditor()
	})
	closeBtn := iconButton(theme.CancelIcon(), T("editor.find.close"), a.close)
	toggles := container.NewHBox(small(38, a.caseBtn), small(38, a.wordBtn), small(38, a.regexBtn))
	findTools := container.NewHBox(toggles, widthBox(78, a.count), prevBtn, nextBtn, allBtn, closeBtn)
	a.findRow = container.NewBorder(nil, nil, nil, findTools, a.find)

	replOne := actionButton(T("editor.find.replace"), nil, a.replaceOne)
	replAll := actionButton(T("editor.find.replaceAll"), nil, a.replaceAll)
	a.replRow = container.NewBorder(nil, nil, nil, container.NewHBox(widthBox(84, replOne), widthBox(56, replAll)), a.repl)

	gotoClose := iconButton(theme.CancelIcon(), T("editor.find.close"), a.close)
	a.gotoRow = container.NewBorder(nil, nil, vCenter(caption(T("editor.goto.label"))), gotoClose, a.gotoLine)

	a.barSlot = container.NewVBox()
	a.barSlot.Hide()

	a.status = widget.NewLabel("")
	a.status.SizeName = theme.SizeNameCaptionText
	a.status.Importance = widget.LowImportance
	a.status.Alignment = fyne.TextAlignTrailing
	statusRow := container.New(layout.NewCustomPaddedLayout(-6, -6, 0, 4), a.status)

	a.root = container.NewBorder(a.barSlot, statusRow, nil, nil, ed)
	a.updateStatus()
	return a
}

func (a *codeArea) focusEditor() {
	if c := a.editor.canvas(); c != nil {
		c.Focus(a.editor)
	}
}

func (a *codeArea) focus(o fyne.Focusable) {
	if c := a.editor.canvas(); c != nil {
		c.Focus(o)
	}
}

func (a *codeArea) show(rows ...fyne.CanvasObject) {
	a.barSlot.Objects = append(rows, widget.NewSeparator())
	a.barSlot.Show()
	// The frame has to lay the bar out again: refreshing the slot alone
	// leaves it at the zero size it had while hidden.
	a.root.Refresh()
}

// openFind shows the find bar, seeded with the selected text.
func (a *codeArea) openFind(replace bool) {
	a.replacing = replace
	if t := a.editor.selectionText(); t != "" && !strings.Contains(t, "\n") {
		a.find.SetText(t)
	}
	a.startOff = a.editor.offset(a.editor.primary().from())
	if replace {
		a.show(a.findRow, a.replRow)
	} else {
		a.show(a.findRow)
	}
	a.runSearch(false)
	a.focus(a.find)
	a.find.TypedShortcut(&fyne.ShortcutSelectAll{})
}

func (a *codeArea) openGoto() {
	a.gotoLine.SetText("")
	a.gotoLine.SetPlaceHolder(T("editor.goto.placeholder", "n", len(a.editor.lines)))
	a.show(a.gotoRow)
	a.focus(a.gotoLine)
}

func (a *codeArea) close() {
	a.barSlot.Hide()
	a.barSlot.Objects = nil
	a.root.Refresh()
	a.editor.clearSearch()
	a.focusEditor()
}

func (a *codeArea) query() searchQuery {
	return searchQuery{
		text:          a.find.Text,
		caseSensitive: a.caseBtn.active,
		wholeWord:     a.wordBtn.active,
		regex:         a.regexBtn.active,
	}
}

// runSearch re-runs the search; incremental moves the selection to the first
// match after where the search started.
func (a *codeArea) runSearch(incremental bool) {
	a.editor.setSearch(a.query())
	if incremental && len(a.editor.matches) > 0 {
		a.editor.findFrom(a.startOff)
	}
	a.updateCount()
}

func (a *codeArea) updateCount() {
	e := a.editor
	switch {
	case e.searchErr != nil:
		a.count.SetText(T("editor.find.badPattern"))
	case a.find.Text == "":
		a.count.SetText("")
	case len(e.matches) == 0:
		a.count.SetText(T("editor.find.noResults"))
	default:
		total := strconv.Itoa(len(e.matches))
		if len(e.matches) >= maxMatches {
			total += "+"
		}
		if i := e.currentMatch(); i >= 0 {
			a.count.SetText(T("editor.find.current", "current", i+1, "total", total))
		} else {
			a.count.SetText(T("editor.find.found", "total", total))
		}
	}
}

func (a *codeArea) next(backward bool) {
	a.editor.findNext(backward)
	a.updateCount()
}

func (a *codeArea) replaceOne() {
	a.editor.replaceCurrent(a.repl.Text)
	a.updateCount()
}

func (a *codeArea) replaceAll() {
	n := a.editor.replaceAll(a.repl.Text)
	a.updateCount()
	if n > 0 {
		a.count.SetText(T("editor.find.replaced", "n", n))
	}
}

// jump goes to the line (and column) typed in the go-to-line bar.
func (a *codeArea) jump() {
	parts := strings.SplitN(strings.TrimSpace(a.gotoLine.Text), ":", 2)
	row, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return
	}
	col := 0
	if len(parts) == 2 {
		if c, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			col = c - 1
		}
	}
	a.close()
	a.editor.SetCursor(row-1, col)
}

// updateStatus writes the caret position and the document facts.
func (a *codeArea) updateStatus() {
	e := a.editor
	parts := []string{T("editor.status.position", "line", e.row+1, "col", e.col+1)}
	if n := len(e.extra) + 1; n > 1 {
		parts = append(parts, T("editor.status.selections", "n", n))
	} else if t := e.selectionText(); t != "" {
		parts = append(parts, T("editor.status.selected", "n", runeLen(t)))
	}
	parts = append(parts, langLabel(e.lang.name), T("editor.status.spaces", "n", e.lang.unit))
	if e.wrap {
		parts = append(parts, T("editor.status.wrap"))
	}
	if editorZoom != 1 {
		parts = append(parts, fmt.Sprintf("%d%%", int(editorZoom*100+0.5)))
	}
	a.status.SetText(strings.Join(parts, "  ·  "))
}

func langLabel(name string) string {
	switch name {
	case "lua":
		return "Luau"
	case "yaml":
		return "YAML"
	case "json":
		return "JSON"
	case "markdown":
		return "Markdown"
	}
	return T("editor.lang.plain")
}
