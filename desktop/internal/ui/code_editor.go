package ui

import (
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The code editor is Studio's own: Fyne has no editor widget and a web view
// cannot be embedded in a Fyne canvas. It is split by concern:
//
//	code_editor.go    the widget, its public API, selections and history
//	editor_edit.go    editing commands (typing, lines, comments, case, words)
//	editor_input.go   keyboard, mouse and the context menu
//	editor_view.go    layout: folding, soft wrap, coordinates, scrolling
//	editor_render.go  drawing
//	editor_lang.go    per-language highlighting, comments, indent and folds
//	editor_find.go    search, the find/replace bar and the codeArea frame

// editorDiagnostic is the code editor's view of one LSP finding.
type editorDiagnostic struct {
	Line, Col       int
	EndLine, EndCol int
	Severity        int
	Message         string
}

type completionItem struct {
	Label  string
	Detail string
	Insert string
}

type completionMenu struct {
	visible  bool
	items    []completionItem
	selected int
	scroll   int
	rows     int
	row      int // where the replaced identifier starts
	col      int
	prefix   string
	x, y     float32
	w, h     float32
	rowH     float32
}

// tpos is a position in the document: a line and a rune column.
type tpos struct{ row, col int }

func (p tpos) before(q tpos) bool { return p.row < q.row || (p.row == q.row && p.col < q.col) }

// selRange is one selection. anchor is where it started, head is the caret;
// they are equal for a bare caret. goal is the visual column vertical motion
// tries to keep (-1 when unset).
type selRange struct {
	anchor, head tpos
	goal         int
}

func caretAt(p tpos) selRange { return selRange{anchor: p, head: p, goal: -1} }

func (s selRange) empty() bool { return s.anchor == s.head }

func (s selRange) from() tpos {
	if s.anchor.before(s.head) {
		return s.anchor
	}
	return s.head
}

func (s selRange) to() tpos {
	if s.anchor.before(s.head) {
		return s.head
	}
	return s.anchor
}

// change replaces the runes [from, to) of the document with text. Offsets
// count runes, with each line break as one.
type change struct {
	from, to int
	text     string
}

// histEntry is one applied transaction: the changes, their inverse, and the
// selections on either side.
type histEntry struct {
	changes, inverse []change
	before, after    []selRange
}

// histGroup is one undo step: several transactions when typing coalesces.
type histGroup struct {
	entries []histEntry
	kind    string
	at      time.Time
}

// codeEditor is the editor widget: a document of lines, any number of
// selections (the primary one mirrors row/col/anchor/sel), change-based undo,
// folding, soft wrap, search highlights, inline diagnostics, hover cards and
// an inline completion menu. The LSP wrapper in luaEditor supplies only
// diagnostics and completion candidates.
type codeEditor struct {
	widget.BaseWidget

	lines    []string
	lang     *langSpec
	readOnly bool

	// The primary selection. sel is false for a bare caret; anchorR/anchorC
	// are meaningful only while it is true.
	row, col         int
	anchorR, anchorC int
	sel              bool
	goal             int
	// extra holds the other selections, in no particular order.
	extra []selRange

	scrollX, scrollY float32
	minRows          int
	focused          bool
	shiftDown        bool
	ctrlDown         bool
	altDown          bool
	caretOn          bool
	caretTimer       *time.Timer
	caretGen         int
	lastInput        time.Time
	destroyed        bool
	width, height    float32

	diags        []editorDiagnostic
	comp         completionMenu
	hoverText    string
	hoverVisible bool
	hoverRow     int
	hoverCol     int

	undos []histGroup
	redos []histGroup

	// version counts document edits; caches compare against it.
	version    int
	lineStarts []int
	startsAt   int

	// Layout: visual lines after folding and wrapping (editor_view.go).
	folds     map[int]bool
	foldGen   int
	wrap      bool
	vls       []vline
	firstV    []int
	layoutKey layoutKey

	// Highlighter states at the start of each line (editor_render.go).
	hlStates []int
	hlValid  int

	// Search (editor_find.go).
	search     searchQuery
	matches    []change
	searchErr  error
	searchSeen int

	// Mouse gestures.
	dragMode     int
	dragAnchor   tpos
	dragBarGrab  float32
	lastDouble   time.Time
	lastDoubleAt int

	onChanged      func()
	onCursor       func()
	onCompletion   func(prefix string, row, startCol int)
	onAutoComplete func()
	onShortcut     func(fyne.Shortcut) bool
	onHover        func(row, col int, overDiag bool)
	onHoverOut     func()
	// onFind opens the find bar (replace mode when true); onGoto the go-to-line
	// bar. codeArea sets them.
	onFind func(replace bool)
	onGoto func()

	surface *fyne.Container
}

// editorZoom is shared by every editor, so Ctrl+wheel in one keeps them all
// the same size.
var editorZoom float32 = 1

func newCodeEditor(source string) *codeEditor {
	return newCodeEditorLang(source, "lua")
}

func newCodeEditorLang(source, lang string) *codeEditor {
	e := &codeEditor{minRows: 18, goal: -1, lang: languageFor(lang), folds: map[int]bool{}}
	e.ExtendBaseWidget(e)
	e.surface = container.NewWithoutLayout()
	e.setText(source)
	return e
}

func (e *codeEditor) CreateRenderer() fyne.WidgetRenderer {
	return &codeEditorRenderer{ed: e}
}

func (e *codeEditor) AcceptsTab() bool { return true }

func (e *codeEditor) Cursor() desktop.Cursor { return desktop.TextCursor }

func (e *codeEditor) MinSize() fyne.Size {
	h := e.lineHeight()*float32(e.minRows) + 14
	if h < 120 {
		h = 120
	}
	return fyne.NewSize(320, h)
}

func (e *codeEditor) SetMinRowsVisible(rows int) {
	if rows > 0 {
		e.minRows = rows
	}
	e.Refresh()
}

// SetLanguage switches the highlighter, comment style, indentation and fold
// rules: lua, yaml, json, markdown or text.
func (e *codeEditor) SetLanguage(lang string) {
	e.lang = languageFor(lang)
	e.hlValid = 0
	e.folds = map[int]bool{}
	e.invalidateLayout()
	e.Refresh()
}

// SetReadOnly lets the text be selected, searched and copied but not edited.
func (e *codeEditor) SetReadOnly(on bool) {
	e.readOnly = on
	e.Refresh()
}

// SetWrap turns soft wrapping on or off.
func (e *codeEditor) SetWrap(on bool) {
	if e.wrap == on {
		return
	}
	e.wrap = on
	e.scrollX = 0
	e.invalidateLayout()
	e.Refresh()
}

func (e *codeEditor) SetText(source string) {
	e.setText(source)
	e.notify()
}

func (e *codeEditor) Text() string { return strings.Join(e.lines, "\n") }

func (e *codeEditor) setText(source string) {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")
	source = strings.ReplaceAll(source, "\t", "  ")
	e.lines = strings.Split(source, "\n")
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	e.row, e.col, e.sel, e.extra, e.goal = 0, 0, false, nil, -1
	e.undos, e.redos = nil, nil
	e.comp = completionMenu{}
	e.folds = map[int]bool{}
	e.bump(0)
	e.clampCursor()
	e.Refresh()
}

func (e *codeEditor) SetDiagnostics(diags []editorDiagnostic) {
	e.diags = diags
	e.Refresh()
}

func (e *codeEditor) CursorPosition() (row, col int) { return e.row, e.col }

// SetCursor collapses every selection to one caret at row:col.
func (e *codeEditor) SetCursor(row, col int) {
	e.touchCaret()
	e.setSels([]selRange{caretAt(tpos{row, col})}, 0)
	e.revealCursor()
	e.comp.visible = false
	e.hoverVisible = false
	e.cursorMoved()
}

// ---- metrics ------------------------------------------------------------

func (e *codeEditor) lineHeight() float32 {
	h := fyne.MeasureText("Mg", e.textSize(), fyne.TextStyle{Monospace: true}).Height
	if h < 14 {
		h = 14
	}
	return h + 3
}

func (e *codeEditor) baseTextSize() float32 {
	if e.Theme() == nil {
		return theme.TextSize()
	}
	return e.Theme().Size(theme.SizeNameText)
}

func (e *codeEditor) textSize() float32 { return e.baseTextSize() * editorZoom }

func (e *codeEditor) captionSize() float32 {
	if e.Theme() == nil {
		return theme.TextSize() - 2
	}
	return e.Theme().Size(theme.SizeNameCaptionText)
}

func (e *codeEditor) charWidth() float32 {
	return fyne.MeasureText("M", e.textSize(), fyne.TextStyle{Monospace: true}).Width
}

func itoa(n int) string {
	if n <= 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// measureRunes is the width of the first n runes of s. Monospace ASCII is a
// multiplication; anything else is measured, so wide glyphs stay aligned.
func measureRunes(s string, n int, size float32) float32 {
	if n <= 0 {
		return 0
	}
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		if n > len(s) {
			n = len(s)
		}
		return float32(n) * fyne.MeasureText("M", size, fyne.TextStyle{Monospace: true}).Width
	}
	r := []rune(s)
	if n > len(r) {
		n = len(r)
	}
	return fyne.MeasureText(string(r[:n]), size, fyne.TextStyle{Monospace: true}).Width
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func (e *codeEditor) lineLen(row int) int { return runeLen(e.lines[row]) }

// ---- offsets ------------------------------------------------------------

func (e *codeEditor) starts() []int {
	if e.lineStarts == nil || e.startsAt != e.version || len(e.lineStarts) != len(e.lines) {
		e.lineStarts = make([]int, len(e.lines))
		off := 0
		for i, l := range e.lines {
			e.lineStarts[i] = off
			off += runeLen(l) + 1
		}
		e.startsAt = e.version
	}
	return e.lineStarts
}

func (e *codeEditor) docLen() int {
	st := e.starts()
	return st[len(st)-1] + e.lineLen(len(e.lines)-1)
}

func (e *codeEditor) offset(p tpos) int {
	p = e.clampPos(p)
	return e.starts()[p.row] + p.col
}

func (e *codeEditor) posAt(off int) tpos {
	st := e.starts()
	if off <= 0 {
		return tpos{}
	}
	row := sort.Search(len(st), func(i int) bool { return st[i] > off }) - 1
	if row < 0 {
		row = 0
	}
	col := off - st[row]
	if l := e.lineLen(row); col > l {
		col = l
	}
	return tpos{row, col}
}

func (e *codeEditor) clampPos(p tpos) tpos {
	if p.row < 0 {
		return tpos{}
	}
	if p.row >= len(e.lines) {
		last := len(e.lines) - 1
		return tpos{last, e.lineLen(last)}
	}
	if p.col < 0 {
		p.col = 0
	}
	if l := e.lineLen(p.row); p.col > l {
		p.col = l
	}
	return p
}

// textRange returns the document text between two offsets.
func (e *codeEditor) textRange(from, to int) string {
	if to <= from {
		return ""
	}
	a, b := e.posAt(from), e.posAt(to)
	if a.row == b.row {
		r := []rune(e.lines[a.row])
		return string(r[a.col:b.col])
	}
	var sb strings.Builder
	sb.WriteString(string([]rune(e.lines[a.row])[a.col:]))
	for r := a.row + 1; r < b.row; r++ {
		sb.WriteByte('\n')
		sb.WriteString(e.lines[r])
	}
	sb.WriteByte('\n')
	sb.WriteString(string([]rune(e.lines[b.row])[:b.col]))
	return sb.String()
}

// mapPos moves an offset of the old document through changes (ascending, in
// old-document offsets). A position inside a replaced range goes to its
// start, or with assoc > 0 to the end of the replacement; an insertion right
// at the position pushes it along only with assoc > 0.
func mapPos(changes []change, pos, assoc int) int {
	delta := 0
	for _, c := range changes {
		if c.from > pos {
			break
		}
		if c.from == pos && c.to == pos {
			if assoc > 0 {
				delta += runeLen(c.text)
				continue
			}
			break
		}
		if c.to <= pos {
			delta += runeLen(c.text) - (c.to - c.from)
			continue
		}
		if assoc > 0 {
			return c.from + delta + runeLen(c.text)
		}
		return c.from + delta
	}
	return pos + delta
}

// ---- selections -----------------------------------------------------------

func (e *codeEditor) primary() selRange {
	head := tpos{e.row, e.col}
	if !e.sel {
		return selRange{anchor: head, head: head, goal: e.goal}
	}
	return selRange{anchor: tpos{e.anchorR, e.anchorC}, head: head, goal: e.goal}
}

// allSels returns every selection with the primary first.
func (e *codeEditor) allSels() []selRange {
	out := make([]selRange, 0, 1+len(e.extra))
	out = append(out, e.primary())
	return append(out, e.extra...)
}

func (e *codeEditor) multi() bool { return len(e.extra) > 0 }

// setSels installs selections: they are clamped, sorted, and overlapping ones
// merged; primary indexes sels.
func (e *codeEditor) setSels(sels []selRange, primary int) {
	if len(sels) == 0 {
		sels = []selRange{caretAt(tpos{e.row, e.col})}
		primary = 0
	}
	if primary < 0 || primary >= len(sels) {
		primary = 0
	}
	type item struct {
		s       selRange
		primary bool
	}
	items := make([]item, len(sels))
	for i, s := range sels {
		s.anchor, s.head = e.clampPos(s.anchor), e.clampPos(s.head)
		items[i] = item{s, i == primary}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].s.from().before(items[j].s.from()) })
	merged := items[:0]
	for _, it := range items {
		if n := len(merged); n > 0 {
			last := &merged[n-1]
			overlap := it.s.from().before(last.s.to()) ||
				(it.s.from() == last.s.to() && (it.s.empty() || last.s.empty())) ||
				(it.s.from() == last.s.from())
			if overlap {
				// Keep the union, oriented like the earlier range.
				to := last.s.to()
				if to.before(it.s.to()) {
					to = it.s.to()
				}
				from := last.s.from()
				if last.s.anchor.before(last.s.head) || last.s.empty() {
					last.s.anchor, last.s.head = from, to
				} else {
					last.s.anchor, last.s.head = to, from
				}
				last.primary = last.primary || it.primary
				continue
			}
		}
		merged = append(merged, it)
	}
	e.extra = e.extra[:0]
	found := false
	for _, it := range merged {
		if it.primary && !found {
			found = true
			e.row, e.col = it.s.head.row, it.s.head.col
			e.anchorR, e.anchorC = it.s.anchor.row, it.s.anchor.col
			e.sel = !it.s.empty()
			e.goal = it.s.goal
			continue
		}
		e.extra = append(e.extra, it.s)
	}
	if !found && len(e.extra) > 0 {
		p := e.extra[0]
		e.extra = e.extra[1:]
		e.row, e.col, e.anchorR, e.anchorC = p.head.row, p.head.col, p.anchor.row, p.anchor.col
		e.sel, e.goal = !p.empty(), p.goal
	}
	if len(e.extra) == 0 {
		e.extra = nil
	}
}

// sortedSels returns every selection in document order and the index of the
// primary one.
func (e *codeEditor) sortedSels() ([]selRange, int) {
	all := e.allSels()
	p := all[0]
	sort.SliceStable(all, func(i, j int) bool { return all[i].from().before(all[j].from()) })
	for i, s := range all {
		if s == p {
			return all, i
		}
	}
	return all, 0
}

func (e *codeEditor) clampCursor() {
	e.setSels(e.allSels(), 0)
}

// selection returns the primary selection's normalized range.
func (e *codeEditor) selection() (sr, sc, er, ec int, ok bool) {
	p := e.primary()
	if p.empty() {
		return 0, 0, 0, 0, false
	}
	f, t := p.from(), p.to()
	return f.row, f.col, t.row, t.col, true
}

func (e *codeEditor) selectionText() string {
	p := e.primary()
	if p.empty() {
		return ""
	}
	return e.textRange(e.offset(p.from()), e.offset(p.to()))
}

// collapseToPrimary drops the extra selections; false when there were none.
func (e *codeEditor) collapseToPrimary() bool {
	if len(e.extra) == 0 {
		return false
	}
	e.extra = nil
	e.Refresh()
	return true
}

// ---- transactions and history --------------------------------------------

// offSel is a selection as rune offsets of the document after a change.
type offSel struct{ a, h, goal int }

func caretOff(off int) offSel { return offSel{off, off, -1} }

// commit applies changes (any order; overlaps and no-ops are dropped; offsets
// of the current document) and installs sels, given as offsets of the new
// document. With sels nil every current selection is mapped through the
// changes. kind names the undo step: "type" and "delete" coalesce with a step
// of the same kind made just before, "" never does, "none" is not recorded.
func (e *codeEditor) commit(changes []change, sels []offSel, primary int, kind string) {
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].from < changes[j].from })
	clean := make([]change, 0, len(changes))
	end := -1
	for _, c := range changes {
		if c.from < end || (c.from == c.to && c.text == "") {
			continue
		}
		clean = append(clean, c)
		end = c.to
	}
	changes = clean
	if e.readOnly {
		changes = nil
	}
	before := e.allSels()
	if sels == nil {
		sels = make([]offSel, len(before))
		for i, s := range before {
			sels[i] = offSel{mapPos(changes, e.offset(s.anchor), 1), mapPos(changes, e.offset(s.head), 1), s.goal}
		}
		primary = 0
	}
	inverse := make([]change, len(changes))
	delta := 0
	for i, c := range changes {
		from := c.from + delta
		inverse[i] = change{from: from, to: from + runeLen(c.text), text: e.textRange(c.from, c.to)}
		delta += runeLen(c.text) - (c.to - c.from)
	}
	if len(changes) > 0 {
		e.applyChanges(changes)
	}
	out := make([]selRange, len(sels))
	for i, s := range sels {
		out[i] = selRange{anchor: e.posAt(s.a), head: e.posAt(s.h), goal: s.goal}
	}
	e.setSels(out, primary)
	if len(changes) > 0 && kind != "none" {
		e.record(histEntry{changes: changes, inverse: inverse, before: before, after: e.allSels()}, kind)
	}
	e.afterEdit()
}

// applyChanges splices changes (ascending, non-overlapping) into the lines.
func (e *codeEditor) applyChanges(changes []change) {
	type span struct {
		from, to tpos
		text     string
	}
	spans := make([]span, len(changes))
	for i, c := range changes {
		spans[i] = span{e.posAt(c.from), e.posAt(c.to), c.text}
	}
	first := len(e.lines)
	for i := len(spans) - 1; i >= 0; i-- {
		s := spans[i]
		head := string([]rune(e.lines[s.from.row])[:s.from.col])
		tail := string([]rune(e.lines[s.to.row])[s.to.col:])
		parts := strings.Split(s.text, "\n")
		parts[0] = head + parts[0]
		parts[len(parts)-1] += tail
		rest := append([]string(nil), e.lines[s.to.row+1:]...)
		e.lines = append(append(e.lines[:s.from.row], parts...), rest...)
		e.shiftFolds(s.from.row, s.to.row, len(parts)-1)
		if s.from.row < first {
			first = s.from.row
		}
		// Later spans were converted before this splice; earlier ones are
		// unaffected because they come first in the document.
	}
	e.bump(first)
}

// bump marks the document changed from row on.
func (e *codeEditor) bump(row int) {
	e.version++
	if row < e.hlValid {
		e.hlValid = row
	}
	e.invalidateLayout()
	if e.search.text != "" {
		e.refreshMatches()
	}
}

func (e *codeEditor) record(entry histEntry, kind string) {
	now := time.Now()
	e.redos = nil
	if n := len(e.undos); n > 0 && kind != "" && e.undos[n-1].kind == kind && now.Sub(e.undos[n-1].at) < 700*time.Millisecond {
		g := &e.undos[n-1]
		g.entries = append(g.entries, entry)
		g.at = now
		return
	}
	e.undos = append(e.undos, histGroup{entries: []histEntry{entry}, kind: kind, at: now})
	if len(e.undos) > 500 {
		e.undos = e.undos[len(e.undos)-500:]
	}
}

// breakUndo ends typing coalescence, so the next edit is its own step.
func (e *codeEditor) breakUndo() {
	if n := len(e.undos); n > 0 {
		e.undos[n-1].kind = ""
	}
}

func (e *codeEditor) undo() {
	n := len(e.undos)
	if n == 0 {
		return
	}
	g := e.undos[n-1]
	e.undos = e.undos[:n-1]
	for i := len(g.entries) - 1; i >= 0; i-- {
		e.applyChanges(g.entries[i].inverse)
	}
	e.setSels(g.entries[0].before, 0)
	e.redos = append(e.redos, g)
	e.afterEdit()
}

func (e *codeEditor) redo() {
	n := len(e.redos)
	if n == 0 {
		return
	}
	g := e.redos[n-1]
	e.redos = e.redos[:n-1]
	for _, en := range g.entries {
		e.applyChanges(en.changes)
	}
	e.setSels(g.entries[len(g.entries)-1].after, 0)
	g.kind = ""
	e.undos = append(e.undos, g)
	e.afterEdit()
}

// afterEdit settles the view after the document or selections changed.
func (e *codeEditor) afterEdit() {
	e.unfoldAtCarets()
	e.notify()
	e.cursorMoved()
}

func (e *codeEditor) notify() {
	e.touchCaret()
	e.hoverVisible = false
	e.revealCursor()
	if e.onChanged != nil {
		e.onChanged()
	}
	e.Refresh()
}

func (e *codeEditor) cursorMoved() {
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

// ---- caret blink ------------------------------------------------------------

func (e *codeEditor) touchCaret() {
	if e.destroyed {
		return
	}
	e.caretOn = true
	e.lastInput = time.Now()
	if e.caretTimer != nil {
		e.caretTimer.Stop()
		e.caretTimer = nil
	}
	if !e.focused {
		return
	}
	e.caretGen++
	gen := e.caretGen
	e.caretTimer = time.AfterFunc(500*time.Millisecond, func() { e.blinkCaret(gen) })
}

func (e *codeEditor) blinkCaret(gen int) {
	onUI(func() {
		if e.destroyed || !e.focused || gen != e.caretGen {
			return
		}
		if time.Since(e.lastInput) >= 400*time.Millisecond {
			e.caretOn = !e.caretOn
		} else {
			e.caretOn = true
		}
		e.Refresh()
		if !e.destroyed && e.focused {
			e.caretTimer = time.AfterFunc(530*time.Millisecond, func() { e.blinkCaret(gen) })
		}
	})
}

func (e *codeEditor) stopCaret() {
	e.destroyed = true
	e.focused = false
	e.shiftDown, e.ctrlDown, e.altDown = false, false, false
	e.caretOn = false
	if e.caretTimer != nil {
		e.caretTimer.Stop()
		e.caretTimer = nil
	}
}

func (e *codeEditor) FocusGained() {
	e.focused = true
	e.caretOn = true
	e.touchCaret()
	e.Refresh()
}

func (e *codeEditor) FocusLost() {
	e.focused = false
	e.shiftDown, e.ctrlDown, e.altDown = false, false, false
	e.caretOn = false
	if e.caretTimer != nil {
		e.caretTimer.Stop()
		e.caretTimer = nil
	}
	e.comp.visible = false
	e.Refresh()
}

func (e *codeEditor) canvas() fyne.Canvas {
	if e.app() == nil || e.app().Driver() == nil {
		return nil
	}
	return e.app().Driver().CanvasForObject(e)
}

func (e *codeEditor) app() fyne.App { return fyne.CurrentApp() }

func (e *codeEditor) clipboard() fyne.Clipboard { return fyne.CurrentApp().Clipboard() }

// ---- hover and completion API ---------------------------------------------

// ShowHover displays a symbol card anchored at the caret, in the editor rather
// than in a strip below it.
func (e *codeEditor) ShowHover(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		e.HideHover()
		return
	}
	e.hoverText = text
	e.hoverVisible = true
	e.comp.visible = false
	e.Refresh()
}

func (e *codeEditor) HideHover() {
	if !e.hoverVisible {
		return
	}
	e.hoverVisible = false
	e.Refresh()
}

// RequestCompletion asks the language service for candidates. Automatic
// completion (manual=false) is skipped unless the caret is after an identifier;
// Ctrl+Space (manual=true) always asks.
func (e *codeEditor) RequestCompletion(manual bool) {
	if e.onCompletion == nil {
		return
	}
	line := []rune(e.lines[e.row])
	col := minInt(e.col, len(line))
	start := col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	prefix := string(line[start:col])
	if prefix == "" && !manual {
		// After a field access, suggest members even with no prefix typed.
		afterDot := col > 0 && (line[col-1] == '.' || line[col-1] == ':')
		if !afterDot {
			e.comp.visible = false
			return
		}
	}
	e.onCompletion(prefix, e.row, start)
}

// ShowCompletions installs candidates from the language service. The prefix is
// the identifier already typed; only matching labels are shown.
func (e *codeEditor) ShowCompletions(items []completionItem, prefix string, row, startCol int) {
	if len(items) == 0 {
		e.comp = completionMenu{}
		e.Refresh()
		return
	}
	low := strings.ToLower(prefix)
	filtered := make([]completionItem, 0, len(items))
	for _, it := range items {
		if low == "" || strings.HasPrefix(strings.ToLower(it.Label), low) {
			filtered = append(filtered, it)
		}
	}
	if len(filtered) == 0 {
		e.comp = completionMenu{}
		e.Refresh()
		return
	}
	if len(filtered) > 60 {
		filtered = filtered[:60]
	}
	e.comp = completionMenu{visible: true, items: filtered, row: row, col: startCol, prefix: prefix}
	e.hoverVisible = false
	e.Refresh()
}

// applyCompletion replaces the typed prefix with the chosen candidate. Every
// other caret whose preceding text matches the prefix gets the same insertion.
func (e *codeEditor) applyCompletion() {
	if !e.comp.visible || len(e.comp.items) == 0 {
		return
	}
	it := e.comp.items[e.comp.selected]
	text := stripSnippet(it.Insert)
	if text == "" {
		text = it.Label
	}
	plen := runeLen(e.comp.prefix)
	start := e.offset(tpos{e.comp.row, e.comp.col})
	changes := []change{{from: start, to: minInt(start+plen, e.docLen()), text: text}}
	for _, s := range e.extra {
		if !s.empty() {
			continue
		}
		h := e.offset(s.head)
		if h-plen >= 0 && e.textRange(h-plen, h) == e.comp.prefix {
			changes = append(changes, change{from: h - plen, to: h, text: text})
		}
	}
	e.comp = completionMenu{}
	e.breakUndo()
	e.commitCarets(changes, start, "")
	if c := e.canvas(); c != nil {
		c.Focus(e)
	}
}

// commitCarets applies changes and leaves one caret at the end of each; the
// change starting at primaryFrom carries the primary caret.
func (e *codeEditor) commitCarets(changes []change, primaryFrom int, kind string) {
	sort.SliceStable(changes, func(i, j int) bool { return changes[i].from < changes[j].from })
	sels := make([]offSel, 0, len(changes))
	primary := 0
	delta := 0
	for i, c := range changes {
		end := c.from + delta + runeLen(c.text)
		sels = append(sels, caretOff(end))
		if c.from == primaryFrom {
			primary = i
		}
		delta += runeLen(c.text) - (c.to - c.from)
	}
	e.commit(changes, sels, primary, kind)
}
