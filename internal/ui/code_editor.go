package ui

import (
	"image/color"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

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

type editorSnapshot struct {
	lines    []string
	row, col int
	anchorR  int
	anchorC  int
	sel      bool
}

// codeEditor is a small code editor widget: syntax-highlighted Lua, a gutter,
// current-line/selection rendering, inline diagnostics with an error lens, and
// an inline completion menu. Editing state is owned here; the LSP wrapper in
// luaEditor only supplies diagnostics and completion candidates.
type codeEditor struct {
	widget.BaseWidget

	lines []string
	row   int
	col   int
	// Selection is anchored at (anchorR, anchorC); sel is false when empty.
	anchorR, anchorC int
	sel              bool

	scrollX, scrollY float32
	minRows          int
	focused          bool
	shiftDown        bool
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

	undos []editorSnapshot
	redos []editorSnapshot
	// Typing coalesces into one undo step; a pause starts a new one.
	lastEdit time.Time
	typing   bool

	onChanged      func()
	onCursor       func()
	onCompletion   func(prefix string, row, startCol int)
	onAutoComplete func()
	onShortcut     func(fyne.Shortcut) bool
	onHover        func(row, col int, overDiag bool)
	onHoverOut     func()

	surface *fyne.Container
}

func newCodeEditor(source string) *codeEditor {
	e := &codeEditor{minRows: 18}
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
	if h < 160 {
		h = 160
	}
	return fyne.NewSize(520, h)
}

func (e *codeEditor) SetMinRowsVisible(rows int) {
	if rows > 0 {
		e.minRows = rows
	}
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
	e.row, e.col = 0, 0
	e.sel = false
	e.undos, e.redos = nil, nil
	e.comp = completionMenu{}
	e.clampCursor()
	e.Refresh()
}

func (e *codeEditor) SetDiagnostics(diags []editorDiagnostic) {
	e.diags = diags
	e.Refresh()
}

func (e *codeEditor) CursorPosition() (row, col int) { return e.row, e.col }

func (e *codeEditor) SetCursor(row, col int) {
	e.touchCaret()
	e.row, e.col = row, col
	e.clampCursor()
	e.sel = false
	e.ensureCursorVisible()
	e.comp.visible = false
	e.hoverVisible = false
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func (e *codeEditor) lineHeight() float32 {
	h := fyne.MeasureText("Mg", e.textSize(), fyne.TextStyle{Monospace: true}).Height
	if h < 14 {
		h = 14
	}
	return h + 3
}

func (e *codeEditor) textSize() float32 {
	if e.Theme() == nil {
		return theme.TextSize()
	}
	return e.Theme().Size(theme.SizeNameText)
}

func (e *codeEditor) captionSize() float32 {
	if e.Theme() == nil {
		return theme.TextSize() - 2
	}
	return e.Theme().Size(theme.SizeNameCaptionText)
}

func (e *codeEditor) charWidth() float32 {
	return fyne.MeasureText("M", e.textSize(), fyne.TextStyle{Monospace: true}).Width
}

func (e *codeEditor) gutterWidth() float32 {
	digits := len(itoa(len(e.lines)))
	return float32(digits)*e.charWidth() + 22
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

func (e *codeEditor) clampCursor() {
	if e.row < 0 {
		e.row = 0
	}
	if e.row >= len(e.lines) {
		e.row = len(e.lines) - 1
	}
	line := []rune(e.lines[e.row])
	if e.col < 0 {
		e.col = 0
	}
	if e.col > len(line) {
		e.col = len(line)
	}
	if e.anchorR < 0 {
		e.anchorR = 0
	}
	if e.anchorR >= len(e.lines) {
		e.anchorR = len(e.lines) - 1
	}
	al := []rune(e.lines[e.anchorR])
	if e.anchorC < 0 {
		e.anchorC = 0
	}
	if e.anchorC > len(al) {
		e.anchorC = len(al)
	}
}

// selection returns the normalized selection range.
func (e *codeEditor) selection() (sr, sc, er, ec int, ok bool) {
	if !e.sel || (e.anchorR == e.row && e.anchorC == e.col) {
		return 0, 0, 0, 0, false
	}
	if e.anchorR < e.row || (e.anchorR == e.row && e.anchorC < e.col) {
		return e.anchorR, e.anchorC, e.row, e.col, true
	}
	return e.row, e.col, e.anchorR, e.anchorC, true
}

func (e *codeEditor) selectionText() string {
	sr, sc, er, ec, ok := e.selection()
	if !ok {
		return ""
	}
	if sr == er {
		r := []rune(e.lines[sr])
		return string(r[sc:minInt(ec, len(r))])
	}
	parts := []string{string([]rune(e.lines[sr])[sc:])}
	for r := sr + 1; r < er; r++ {
		parts = append(parts, e.lines[r])
	}
	parts = append(parts, string([]rune(e.lines[er])[:minInt(ec, len([]rune(e.lines[er])))]))
	return strings.Join(parts, "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (e *codeEditor) pushUndo(typing bool) {
	now := time.Now()
	if typing && e.typing && now.Sub(e.lastEdit) < 700*time.Millisecond {
		e.lastEdit = now
		e.redos = nil
		return
	}
	e.typing = typing
	e.lastEdit = now
	e.redos = nil
	snap := editorSnapshot{lines: append([]string(nil), e.lines...), row: e.row, col: e.col,
		anchorR: e.anchorR, anchorC: e.anchorC, sel: e.sel}
	e.undos = append(e.undos, snap)
	if len(e.undos) > 200 {
		e.undos = e.undos[len(e.undos)-200:]
	}
}

func (e *codeEditor) restore(s editorSnapshot) {
	e.lines = append([]string(nil), s.lines...)
	e.row, e.col, e.anchorR, e.anchorC, e.sel = s.row, s.col, s.anchorR, s.anchorC, s.sel
	e.clampCursor()
	e.ensureCursorVisible()
	e.notify()
	if e.onCursor != nil {
		e.onCursor()
	}
}

func (e *codeEditor) undo() {
	if len(e.undos) == 0 {
		return
	}
	cur := editorSnapshot{lines: append([]string(nil), e.lines...), row: e.row, col: e.col,
		anchorR: e.anchorR, anchorC: e.anchorC, sel: e.sel}
	e.redos = append(e.redos, cur)
	s := e.undos[len(e.undos)-1]
	e.undos = e.undos[:len(e.undos)-1]
	e.typing = false
	e.restore(s)
}

func (e *codeEditor) redo() {
	if len(e.redos) == 0 {
		return
	}
	cur := editorSnapshot{lines: append([]string(nil), e.lines...), row: e.row, col: e.col,
		anchorR: e.anchorR, anchorC: e.anchorC, sel: e.sel}
	e.undos = append(e.undos, cur)
	s := e.redos[len(e.redos)-1]
	e.redos = e.redos[:len(e.redos)-1]
	e.typing = false
	e.restore(s)
}

func (e *codeEditor) deleteSelection() bool {
	sr, sc, er, ec, ok := e.selection()
	if !ok {
		return false
	}
	if sr == er {
		r := []rune(e.lines[sr])
		e.lines[sr] = string(r[:sc]) + string(r[minInt(ec, len(r)):])
	} else {
		first := []rune(e.lines[sr])
		last := []rune(e.lines[er])
		e.lines[sr] = string(first[:sc]) + string(last[minInt(ec, len(last)):])
		e.lines = append(e.lines[:sr+1], e.lines[er+1:]...)
	}
	e.row, e.col = sr, sc
	e.sel = false
	e.clampCursor()
	return true
}

func (e *codeEditor) replaceSelection(text string) {
	e.pushUndo(false)
	e.deleteSelection()
	e.insertRaw(text)
}

func (e *codeEditor) insertRaw(text string) {
	parts := strings.Split(text, "\n")
	line := []rune(e.lines[e.row])
	before := string(line[:e.col])
	after := string(line[minInt(e.col, len(line)):])
	if len(parts) == 1 {
		e.lines[e.row] = before + parts[0] + after
		e.col += len([]rune(parts[0]))
		return
	}
	newLines := []string{before + parts[0]}
	newLines = append(newLines, parts[1:len(parts)-1]...)
	newLines = append(newLines, parts[len(parts)-1]+after)
	e.lines = append(e.lines[:e.row], append(newLines, e.lines[e.row+1:]...)...)
	e.row += len(parts) - 1
	e.col = len([]rune(parts[len(parts)-1]))
}

func (e *codeEditor) notify() {
	e.touchCaret()
	e.hoverVisible = false
	e.clampCursor()
	e.ensureCursorVisible()
	if e.onChanged != nil {
		e.onChanged()
	}
	e.Refresh()
}

func (e *codeEditor) insert(r rune) {
	e.pushUndo(true)
	e.sel = false
	if r == '\n' {
		e.newline()
		e.notify()
		return
	}
	if e.handleAutoPair(r) {
		e.notify()
		return
	}
	e.insertRaw(string(r))
	e.notify()
	if e.onAutoComplete != nil && (isWordRune(r) || r == '.' || r == ':') {
		e.onAutoComplete()
	}
}

func (e *codeEditor) handleAutoPair(r rune) bool {
	line := []rune(e.lines[e.row])
	if r == '"' || r == '\'' {
		if e.col < len(line) && line[e.col] == r {
			e.col++
			return true
		}
		if e.col > 0 && isWordRune(line[e.col-1]) {
			e.insertRaw(string(r))
			return true
		}
		e.insertRaw(string([]rune{r, r}))
		e.col--
		return true
	}
	if strings.ContainsRune("([{", r) {
		pair := map[rune]rune{'(': ')', '[': ']', '{': '}'}[r]
		e.insertRaw(string([]rune{r, pair}))
		e.col--
		return true
	}
	if strings.ContainsRune(")]}", r) {
		if e.col < len(line) && line[e.col] == r {
			e.col++
			return true
		}
		e.insertRaw(string(r))
		return true
	}
	return false
}

func (e *codeEditor) newline() {
	line := e.lines[e.row]
	runes := []rune(line)
	lead := leadingSpaces(line)
	indent := strings.Repeat(" ", lead)
	extra := ""
	trimmed := strings.TrimSpace(string(runes[:e.col]))
	if strings.HasSuffix(trimmed, "then") || strings.HasSuffix(trimmed, "do") ||
		strings.HasSuffix(trimmed, "function") || strings.HasSuffix(trimmed, "repeat") ||
		strings.HasSuffix(trimmed, "else") || strings.HasSuffix(trimmed, "{") ||
		(strings.HasPrefix(strings.TrimSpace(trimmed), "function") && strings.HasSuffix(trimmed, ")")) {
		extra = "  "
	}
	indent += extra
	if e.col < len(runes) && runes[e.col] == ')' {
		// Keep a closing call on its own indentation level.
		indent = strings.Repeat(" ", lead)
	}
	e.insertRaw("\n" + indent)
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

func (e *codeEditor) backspace() {
	if _, _, _, _, ok := e.selection(); ok {
		e.pushUndo(false)
		e.deleteSelection()
		e.notify()
		return
	}
	e.pushUndo(false)
	if e.col == 0 {
		if e.row == 0 {
			return
		}
		prev := []rune(e.lines[e.row-1])
		cur := e.lines[e.row]
		e.lines[e.row-1] = string(prev) + cur
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
		e.col = len(prev)
		e.notify()
		return
	}
	line := []rune(e.lines[e.row])
	// Delete an empty auto-paired opening and its closer together.
	if e.col < len(line) && e.col > 0 {
		open, close := line[e.col-1], line[e.col]
		if (open == '(' && close == ')') || (open == '[' && close == ']') ||
			(open == '{' && close == '}') || (open == '"' && close == '"') || (open == '\'' && close == '\'') {
			e.lines[e.row] = string(line[:e.col-1]) + string(line[e.col+1:])
			e.col--
			e.notify()
			return
		}
	}
	e.lines[e.row] = string(line[:e.col-1]) + string(line[e.col:])
	e.col--
	e.notify()
}

func (e *codeEditor) deleteForward() {
	if _, _, _, _, ok := e.selection(); ok {
		e.pushUndo(false)
		e.deleteSelection()
		e.notify()
		return
	}
	e.pushUndo(false)
	line := []rune(e.lines[e.row])
	if e.col < len(line) {
		e.lines[e.row] = string(line[:e.col]) + string(line[e.col+1:])
	} else if e.row < len(e.lines)-1 {
		e.lines[e.row] = e.lines[e.row] + e.lines[e.row+1]
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	}
	e.notify()
}

func (e *codeEditor) indent(outdent bool) {
	sr, _, er, _, sel := e.selection()
	if !sel {
		if outdent {
			e.outdentLine()
		} else {
			e.pushUndo(false)
			e.insertRaw("  ")
			e.notify()
		}
		return
	}
	e.pushUndo(false)
	for r := sr; r <= er; r++ {
		if outdent {
			if strings.HasPrefix(e.lines[r], "  ") {
				e.lines[r] = e.lines[r][2:]
			}
		} else {
			e.lines[r] = "  " + e.lines[r]
		}
	}
	e.anchorC += 2
	e.col += 2
	e.notify()
}

func (e *codeEditor) outdentLine() {
	e.pushUndo(false)
	if strings.HasPrefix(e.lines[e.row], "  ") {
		e.lines[e.row] = e.lines[e.row][2:]
		if e.col >= 2 {
			e.col -= 2
		}
	}
	e.notify()
}

func (e *codeEditor) move(dRow, dCol int, extend bool) {
	e.touchCaret()
	if extend {
		if !e.sel {
			e.anchorR, e.anchorC = e.row, e.col
			e.sel = true
		}
	} else {
		e.sel = false
	}
	e.row += dRow
	e.col += dCol
	if dRow != 0 {
		e.col = minInt(e.col, len([]rune(e.lines[minInt(maxInt(e.row, 0), len(e.lines)-1)])))
	}
	e.clampCursor()
	e.ensureCursorVisible()
	e.comp.visible = false
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (e *codeEditor) home(extend bool) {
	e.touchCaret()
	if extend && !e.sel {
		e.anchorR, e.anchorC = e.row, e.col
		e.sel = true
	} else if !extend {
		e.sel = false
	}
	line := []rune(e.lines[e.row])
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if e.col == i {
		i = 0
	}
	e.col = i
	e.ensureCursorVisible()
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func (e *codeEditor) end(extend bool) {
	e.touchCaret()
	if extend && !e.sel {
		e.anchorR, e.anchorC = e.row, e.col
		e.sel = true
	} else if !extend {
		e.sel = false
	}
	e.col = len([]rune(e.lines[e.row]))
	e.ensureCursorVisible()
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func (e *codeEditor) wordMove(left bool, extend bool) {
	e.touchCaret()
	if extend && !e.sel {
		e.anchorR, e.anchorC = e.row, e.col
		e.sel = true
	} else if !extend {
		e.sel = false
	}
	line := []rune(e.lines[e.row])
	if left {
		for e.col > 0 && !isWordRune(line[e.col-1]) {
			e.col--
		}
		for e.col > 0 && isWordRune(line[e.col-1]) {
			e.col--
		}
	} else {
		for e.col < len(line) && isWordRune(line[e.col]) {
			e.col++
		}
		for e.col < len(line) && !isWordRune(line[e.col]) {
			e.col++
		}
	}
	e.ensureCursorVisible()
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (e *codeEditor) maxScrollY(viewH float32) float32 {
	if viewH <= 0 {
		viewH = e.MinSize().Height
	}
	maxY := float32(len(e.lines))*e.lineHeight() + 8 - viewH
	if maxY < 0 {
		return 0
	}
	return maxY
}

func (e *codeEditor) clampScroll(viewH float32) {
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	if maxY := e.maxScrollY(viewH); e.scrollY > maxY {
		e.scrollY = maxY
	}
}

func (e *codeEditor) ensureCursorVisible() {
	lh := e.lineHeight()
	viewH := e.height
	if viewH <= 0 {
		viewH = e.MinSize().Height
	}
	top := float32(e.row)*lh + 4
	bottom := top + lh
	if top < e.scrollY {
		e.scrollY = top
	}
	if bottom > e.scrollY+viewH-4 {
		e.scrollY = bottom - viewH + 8
	}
	if e.scrollY < 0 {
		e.scrollY = 0
	}
	gw := e.gutterWidth()
	x := e.textX() + measureRunes(e.lines[e.row], e.col, e.textSize())
	viewW := e.width
	if viewW <= 0 {
		viewW = e.MinSize().Width
	}
	if x < gw+12 {
		e.scrollX = 0
	}
	if x > e.width-24 {
		e.scrollX += x - (e.width - 24)
	}
	if e.scrollX < 0 {
		e.scrollX = 0
	}
}

func (e *codeEditor) textX() float32 {
	return e.gutterWidth() + 8 - e.scrollX
}

func measureRunes(s string, n int, size float32) float32 {
	r := []rune(s)
	if n > len(r) {
		n = len(r)
	}
	return fyne.MeasureText(string(r[:n]), size, fyne.TextStyle{Monospace: true}).Width
}

// TypedRune is called for printable input while the editor is focused.
func (e *codeEditor) TypedRune(r rune) {
	if r == '\n' {
		e.insert(r)
		return
	}
	e.insert(r)
}

func (e *codeEditor) deleteWordLeft() {
	if _, _, _, _, ok := e.selection(); ok {
		e.pushUndo(false)
		e.deleteSelection()
		e.notify()
		return
	}
	e.pushUndo(false)
	if e.col == 0 {
		if e.row == 0 {
			return
		}
		prev := []rune(e.lines[e.row-1])
		e.lines[e.row-1] = string(prev) + e.lines[e.row]
		e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
		e.row--
		e.col = len(prev)
		e.notify()
		return
	}
	line := []rune(e.lines[e.row])
	start := e.col
	for start > 0 && unicode.IsSpace(line[start-1]) {
		start--
	}
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	if start == e.col && start > 0 {
		start--
	}
	e.lines[e.row] = string(line[:start]) + string(line[e.col:])
	e.col = start
	e.notify()
}

func (e *codeEditor) deleteWordRight() {
	if _, _, _, _, ok := e.selection(); ok {
		e.pushUndo(false)
		e.deleteSelection()
		e.notify()
		return
	}
	line := []rune(e.lines[e.row])
	if e.col >= len(line) {
		if e.row >= len(e.lines)-1 {
			return
		}
		e.pushUndo(false)
		e.lines[e.row] = e.lines[e.row] + e.lines[e.row+1]
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
		e.notify()
		return
	}
	e.pushUndo(false)
	end := e.col
	for end < len(line) && unicode.IsSpace(line[end]) {
		end++
	}
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	for end < len(line) && unicode.IsSpace(line[end]) {
		end++
	}
	if end == e.col {
		end++
	}
	e.lines[e.row] = string(line[:e.col]) + string(line[end:])
	e.notify()
}

func (e *codeEditor) selectAll() {
	if len(e.lines) == 0 {
		return
	}
	e.touchCaret()
	e.anchorR, e.anchorC = 0, 0
	e.row = len(e.lines) - 1
	e.col = len([]rune(e.lines[e.row]))
	e.sel = e.row != 0 || e.col != 0
	e.comp.visible = false
	e.ensureCursorVisible()
	e.Refresh()
}

func (e *codeEditor) copySelection() {
	if t := e.selectionText(); t != "" {
		e.clipboard().SetContent(t)
	}
}

func (e *codeEditor) cutSelection() {
	if t := e.selectionText(); t != "" {
		e.clipboard().SetContent(t)
		e.pushUndo(false)
		e.deleteSelection()
		e.notify()
	}
}

func (e *codeEditor) paste() {
	text := e.clipboard().Content()
	if text == "" {
		return
	}
	e.replaceSelection(text)
	e.notify()
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

func (e *codeEditor) TypedKey(key *fyne.KeyEvent) {
	switch key.Name {
	case fyne.KeyEscape:
		if e.comp.visible {
			e.comp.visible = false
			e.Refresh()
		} else if e.onShortcut != nil {
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
		rows := int(e.height/e.lineHeight()) - 1
		if rows < 1 {
			rows = 1
		}
		e.move(-rows, 0, e.shiftDown)
	case fyne.KeyPageDown:
		rows := int(e.height/e.lineHeight()) - 1
		if rows < 1 {
			rows = 1
		}
		e.move(rows, 0, e.shiftDown)
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
	}
}

func (e *codeEditor) KeyDown(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftDown = true
	}
}

func (e *codeEditor) KeyUp(key *fyne.KeyEvent) {
	if key.Name == desktop.KeyShiftLeft || key.Name == desktop.KeyShiftRight {
		e.shiftDown = false
	}
}

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
	fyne.Do(func() {
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
	e.shiftDown = false
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
	e.shiftDown = false
	e.caretOn = false
	if e.caretTimer != nil {
		e.caretTimer.Stop()
		e.caretTimer = nil
	}
	e.comp.visible = false
	e.Refresh()
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
	ctrl := cs.Modifier&fyne.KeyModifierControl != 0
	shift := cs.Modifier&fyne.KeyModifierShift != 0
	alt := cs.Modifier&fyne.KeyModifierAlt != 0
	if alt {
		// Alt+Up/Down moves the current line, like VS Code.
		if cs.KeyName == fyne.KeyUp {
			e.moveLine(-1)
			return
		}
		if cs.KeyName == fyne.KeyDown {
			e.moveLine(1)
			return
		}
	}
	if ctrl {
		switch cs.KeyName {
		case fyne.KeyZ:
			if shift {
				e.redo()
			} else {
				e.undo()
			}
			return
		case fyne.KeyY:
			e.redo()
			return
		case fyne.KeyA:
			e.selectAll()
			return
		case fyne.KeyC:
			e.copySelection()
			return
		case fyne.KeyX:
			e.cutSelection()
			return
		case fyne.KeyV:
			e.paste()
			return
		case fyne.KeySpace:
			e.RequestCompletion(true)
			return
		case fyne.KeyBackspace:
			e.deleteWordLeft()
			return
		case fyne.KeyDelete:
			e.deleteWordRight()
			return
		case fyne.KeyLeft:
			e.wordMove(true, shift)
			return
		case fyne.KeyRight:
			e.wordMove(false, shift)
			return
		case fyne.KeyHome:
			if shift {
				if !e.sel {
					e.anchorR, e.anchorC = e.row, e.col
				}
				e.sel = true
			} else {
				e.sel = false
			}
			e.row, e.col = 0, 0
			e.ensureCursorVisible()
			e.Refresh()
			return
		case fyne.KeyEnd:
			if shift {
				if !e.sel {
					e.anchorR, e.anchorC = e.row, e.col
				}
				e.sel = true
			} else {
				e.sel = false
			}
			e.row = len(e.lines) - 1
			e.col = len([]rune(e.lines[e.row]))
			e.ensureCursorVisible()
			e.Refresh()
			return
		}
	}
	if shift {
		switch cs.KeyName {
		case fyne.KeyLeft:
			e.move(0, -1, true)
			return
		case fyne.KeyRight:
			e.move(0, 1, true)
			return
		case fyne.KeyUp:
			e.move(-1, 0, true)
			return
		case fyne.KeyDown:
			e.move(1, 0, true)
			return
		case fyne.KeyHome:
			e.home(true)
			return
		case fyne.KeyEnd:
			e.end(true)
			return
		case fyne.KeyTab:
			e.indent(true)
			return
		}
	}
	if e.onShortcut != nil {
		e.onShortcut(cs)
	}
}

func (e *codeEditor) clipboard() fyne.Clipboard {
	return fyne.CurrentApp().Clipboard()
}

func (e *codeEditor) moveLine(dir int) {
	if dir < 0 && e.row == 0 || dir > 0 && e.row == len(e.lines)-1 {
		return
	}
	e.pushUndo(false)
	other := e.row + dir
	e.lines[e.row], e.lines[other] = e.lines[other], e.lines[e.row]
	e.row = other
	e.sel = false
	e.ensureCursorVisible()
	e.notify()
}

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
	row, col := e.posToCursor(m.Position)
	if m.Modifier&fyne.KeyModifierShift != 0 {
		if !e.sel {
			e.anchorR, e.anchorC = e.row, e.col
			e.sel = true
		}
	} else {
		e.sel = false
		e.anchorR, e.anchorC = row, col
	}
	e.row, e.col = row, col
	e.comp.visible = false
	e.hoverVisible = false
	e.ensureCursorVisible()
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func (e *codeEditor) MouseUp(*desktop.MouseEvent) {}

// overDiagnostic checks the actual painted squiggle, not just the column. A
// pointer in the gap before a glyph maps to that glyph's column but is not
// over the red line, so it must not open the error card.
func (e *codeEditor) overDiagnostic(row, col int, x float32) bool {
	if row < 0 || row >= len(e.lines) {
		return false
	}
	line := []rune(e.lines[row])
	for _, d := range e.diags {
		if d.Line != row {
			continue
		}
		start, end := d.Col, d.EndCol
		if d.EndLine != row {
			end = len(line)
		} else if end <= start {
			end = start + 1
		}
		if start < 0 {
			start = 0
		}
		if end > len(line) {
			end = len(line)
		}
		if col < start || col >= end {
			continue
		}
		x1 := e.textX() + measureRunes(e.lines[row], start, e.textSize())
		x2 := e.textX() + measureRunes(e.lines[row], end, e.textSize())
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
// the pointer, matching the behaviour of the old Entry and of a desktop IDE.
func (e *codeEditor) DoubleTapped(p *fyne.PointEvent) {
	if p == nil {
		return
	}
	if c := e.canvas(); c != nil {
		c.Focus(e)
	}
	row, col := e.posToCursor(p.Position)
	e.selectWord(row, col)
}

func (e *codeEditor) selectWord(row, col int) {
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	line := []rune(e.lines[row])
	if len(line) == 0 {
		e.row, e.col = row, 0
		e.anchorR, e.anchorC = row, 0
		e.sel = false
		e.Refresh()
		return
	}
	if col < 0 {
		col = 0
	}
	if col >= len(line) {
		col = len(line) - 1
	}
	if unicode.IsSpace(line[col]) {
		start, end := col, col+1
		for start > 0 && unicode.IsSpace(line[start-1]) {
			start--
		}
		for end < len(line) && unicode.IsSpace(line[end]) {
			end++
		}
		e.touchCaret()
		e.row, e.col = row, end
		e.anchorR, e.anchorC = row, start
		e.sel = true
		e.comp.visible = false
		e.hoverVisible = false
		e.ensureCursorVisible()
		if e.onCursor != nil {
			e.onCursor()
		}
		e.Refresh()
		return
	}
	if !isWordRune(line[col]) {
		// A double-click outside an identifier selects that one rune, not a
		// run of punctuation.
		e.row, e.col = row, col+1
		e.anchorR, e.anchorC = row, col
		e.sel = true
		e.comp.visible = false
		e.hoverVisible = false
		e.ensureCursorVisible()
		if e.onCursor != nil {
			e.onCursor()
		}
		e.Refresh()
		return
	}
	start, end := col, col+1
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	for end < len(line) && isWordRune(line[end]) {
		end++
	}
	e.touchCaret()
	e.row, e.col = row, end
	e.anchorR, e.anchorC = row, start
	e.sel = true
	e.comp.visible = false
	e.ensureCursorVisible()
	if e.onCursor != nil {
		e.onCursor()
	}
	e.Refresh()
}

func (e *codeEditor) Dragged(d *fyne.DragEvent) {
	e.touchCaret()
	if !e.sel {
		e.anchorR, e.anchorC = e.row, e.col
	}
	e.sel = true
	e.row, e.col = e.posToCursor(d.Position)
	e.clampCursor()
	e.ensureCursorVisible()
	e.Refresh()
}

func (e *codeEditor) DragEnd() {}

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
	if s.Scrolled.DX != 0 {
		e.scrollX -= s.Scrolled.DX * 12
		if e.scrollX < 0 {
			e.scrollX = 0
		}
	}
	if s.Scrolled.DY != 0 {
		e.scrollY -= s.Scrolled.DY * e.lineHeight()
		e.clampScroll(e.height)
	}
	e.comp.visible = false
	e.hoverVisible = false
	e.Refresh()
}

func (e *codeEditor) canvas() fyne.Canvas {
	if e.app() == nil || e.app().Driver() == nil {
		return nil
	}
	return e.app().Driver().CanvasForObject(e)
}

func (e *codeEditor) app() fyne.App {
	// widget.BaseWidget does not expose the app; the canvas is reached through
	// the theme/current app in this desktop-only client.
	return fyne.CurrentApp()
}

func (e *codeEditor) posToCursor(pos fyne.Position) (row, col int) {
	gw := e.gutterWidth()
	lh := e.lineHeight()
	y := pos.Y - 4 + e.scrollY
	row = int(y / lh)
	if row < 0 {
		row = 0
	}
	if row >= len(e.lines) {
		row = len(e.lines) - 1
	}
	x := pos.X - gw - 8 + e.scrollX
	if x < 0 {
		x = 0
	}
	line := e.lines[row]
	width := float32(0)
	col = len([]rune(line))
	for i, r := range []rune(line) {
		w := fyne.MeasureText(string(r), e.textSize(), fyne.TextStyle{Monospace: true}).Width
		if x < width+w/2 {
			col = i
			break
		}
		width += w
	}
	return row, col
}

// ShowHover displays a one-line symbol card anchored at the caret, in the
// editor rather than in a strip below it.
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
	start := e.col
	for start > 0 && isWordRune(line[start-1]) {
		start--
	}
	prefix := string(line[start:e.col])
	if prefix == "" && !manual {
		// After a field access, suggest members even with no prefix typed.
		afterDot := e.col > 0 && (line[e.col-1] == '.' || line[e.col-1] == ':')
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

func (e *codeEditor) applyCompletion() {
	if !e.comp.visible || len(e.comp.items) == 0 {
		return
	}
	it := e.comp.items[e.comp.selected]
	text := stripSnippet(it.Insert)
	if text == "" {
		text = it.Label
	}
	e.pushUndo(false)
	e.row = e.comp.row
	line := []rune(e.lines[e.row])
	start := e.comp.col
	end := start + len([]rune(e.comp.prefix))
	if start > len(line) {
		start = len(line)
	}
	if end > len(line) {
		end = len(line)
	}
	e.lines[e.row] = string(line[:start]) + string(line[end:])
	e.col = start
	e.sel = false
	e.insertRaw(text)
	e.comp = completionMenu{}
	e.ensureCursorVisible()
	e.notify()
	if e.onCursor != nil {
		e.onCursor()
	}
	if c := e.canvas(); c != nil {
		c.Focus(e)
	}
}

func (e *codeEditor) ghostText() (string, float32) {
	if !e.comp.visible || len(e.comp.items) == 0 {
		return "", 0
	}
	it := e.comp.items[e.comp.selected]
	text := stripSnippet(it.Insert)
	if text == "" {
		text = it.Label
	}
	if !strings.HasPrefix(strings.ToLower(text), strings.ToLower(e.comp.prefix)) {
		return "", 0
	}
	rest := text[len(e.comp.prefix):]
	if rest == "" {
		return "", 0
	}
	return rest, measureRunes(e.lines[e.row], e.comp.col, e.textSize()) + measureRunes(e.comp.prefix, len([]rune(e.comp.prefix)), e.textSize())
}

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

func (e *codeEditor) render(size fyne.Size) {
	if size.Width <= 0 || size.Height <= 0 {
		size = e.MinSize()
	}
	e.width, e.height = size.Width, size.Height
	e.clampScroll(size.Height)
	e.surface.Objects = nil
	lh := e.lineHeight()
	gw := e.gutterWidth()

	bg := canvas.NewRectangle(colorSurface)
	bg.Resize(size)
	e.surface.Add(bg)

	first := int(e.scrollY / lh)
	if first < 0 {
		first = 0
	}
	last := first + int(size.Height/lh) + 2
	if last > len(e.lines) {
		last = len(e.lines)
	}

	cur := canvas.NewRectangle(colorSurface2)
	cur.Move(fyne.NewPos(gw, 4+float32(e.row)*lh-e.scrollY))
	cur.Resize(fyne.NewSize(size.Width-gw, lh))
	e.surface.Add(cur)

	gutter := canvas.NewRectangle(colorSurface2)
	gutter.Resize(fyne.NewSize(gw, size.Height))
	e.surface.Add(gutter)
	divider := canvas.NewRectangle(colorBorder)
	divider.Move(fyne.NewPos(gw, 0))
	divider.Resize(fyne.NewSize(1, size.Height))
	e.surface.Add(divider)

	// Other occurrences of the selected text get a second, quieter shade.
	if _, row, sc, ec, ok := e.selectedOccurrence(); ok {
		needle := e.selectionText()
		for row2 := first; row2 < last; row2++ {
			hay := []rune(e.lines[row2])
			for _, at := range findOccurrences(hay, []rune(needle)) {
				atEnd := at + len([]rune(needle))
				if row2 == row && at < ec && atEnd > sc {
					continue // the active selection already owns this range
				}
				x1 := e.textX() + measureRunes(e.lines[row2], at, e.textSize())
				x2 := e.textX() + measureRunes(e.lines[row2], atEnd, e.textSize())
				oc := canvas.NewRectangle(occurrenceColor())
				oc.Move(fyne.NewPos(maxFloat(gw+1, x1), 4+float32(row2)*lh-e.scrollY))
				oc.Resize(fyne.NewSize(x2-x1, lh))
				e.surface.Add(oc)
			}
		}
	}

	if e.sel {
		sr, sc, er, ec, ok := e.selection()
		if ok {
			sr, sc, er, ec = e.clampSelection(sr, sc, er, ec)
			for row := sr; row <= er && row < len(e.lines); row++ {
				if float32(row)*lh-e.scrollY+lh < 0 || float32(row)*lh-e.scrollY > size.Height {
					continue
				}
				start := 0
				if row == sr {
					start = sc
				}
				end := len([]rune(e.lines[row]))
				if row == er {
					end = ec
				}
				x1 := e.textX() + measureRunes(e.lines[row], start, e.textSize())
				x2 := e.textX() + measureRunes(e.lines[row], end, e.textSize())
				if row < er && end == len([]rune(e.lines[row])) {
					x2 += e.charWidth() / 2
				}
				if x2 < x1 {
					x2 = x1
				}
				rect := canvas.NewRectangle(selectionColor())
				rect.Move(fyne.NewPos(maxFloat(gw+1, x1), 4+float32(row)*lh-e.scrollY))
				rect.Resize(fyne.NewSize(maxFloat(1, x2-x1), lh))
				e.surface.Add(rect)
			}
		}
	}

	textSize := e.textSize()
	for row := first; row < last; row++ {
		y := 4 + float32(row)*lh - e.scrollY
		line := e.lines[row]
		num := canvas.NewText(itoa(row+1), colorFaint)
		num.TextSize = e.captionSize()
		num.TextStyle = fyne.TextStyle{Monospace: true}
		nw := fyne.MeasureText(num.Text, num.TextSize, num.TextStyle).Width
		num.Move(fyne.NewPos(gw-8-nw, y))
		e.surface.Add(num)

		x := e.textX()
		for _, span := range highlightLua(line) {
			if span.text == "" {
				continue
			}
			t := canvas.NewText(span.text, span.color)
			t.TextSize = textSize
			t.TextStyle = fyne.TextStyle{Monospace: true}
			t.Move(fyne.NewPos(x, y))
			e.surface.Add(t)
			x += fyne.MeasureText(span.text, textSize, t.TextStyle).Width
		}
	}

	for _, d := range e.diags {
		if d.Line < first || d.Line >= last {
			continue
		}
		y := 4 + float32(d.Line)*lh - e.scrollY
		marker := canvas.NewRectangle(severityColor(d.Severity))
		marker.Move(fyne.NewPos(4, y))
		marker.Resize(fyne.NewSize(3, lh))
		e.surface.Add(marker)
		start, end := d.Col, d.EndCol
		if start < 0 {
			start = 0
		}
		if d.EndLine != d.Line {
			end = len([]rune(e.lines[d.Line]))
		} else if end <= start {
			end = start + 1
		}
		x1 := e.textX() + measureRunes(e.lines[d.Line], start, textSize)
		x2 := e.textX() + measureRunes(e.lines[d.Line], end, textSize)
		if x2 <= x1 {
			x2 = x1 + e.charWidth()
		}
		// A wavy underline reads as a diagnostic rather than a divider.
		step, amp := float32(3), float32(1.5)
		ybase := y + lh - 3
		up := true
		for sx := x1; sx < x2; sx += step {
			w := step
			if sx+w > x2 {
				w = x2 - sx
			}
			line := canvas.NewLine(severityColor(d.Severity))
			line.StrokeWidth = 1.2
			y1, y2 := ybase, ybase+amp
			if !up {
				y1, y2 = ybase+amp, ybase
			}
			line.Position1 = fyne.NewPos(sx, y1)
			line.Position2 = fyne.NewPos(sx+w, y2)
			e.surface.Add(line)
			up = !up
		}
		// The message itself is shown in the compact hover card when the
		// caret enters the line. Drawing it at the end of every diagnostic
		// line overlaps the code (and other diagnostics), so only the gutter
		// marker and squiggle live in the text canvas.
	}

	if ghost, gx := e.ghostText(); ghost != "" {
		y := 4 + float32(e.row)*lh - e.scrollY
		ghostColor := colorFaint
		ghostColor.A = 190
		t := canvas.NewText(ghost, ghostColor)
		t.TextSize = textSize
		t.TextStyle = fyne.TextStyle{Monospace: true, Italic: true}
		t.Move(fyne.NewPos(e.textX()+gx, y))
		e.surface.Add(t)
	}

	if e.focused && e.caretOn {
		cursor := canvas.NewRectangle(colorAccent)
		cursor.Move(fyne.NewPos(e.textX()+measureRunes(e.lines[e.row], e.col, textSize), 4+float32(e.row)*lh-e.scrollY))
		cursor.Resize(fyne.NewSize(1, lh-2))
		e.surface.Add(cursor)
	}

	if e.hoverVisible && !e.comp.visible {
		e.renderHover(size, lh)
	}
	if e.comp.visible {
		e.renderCompletion(size, gw, lh)
	}
	e.surface.Refresh()
}

func (e *codeEditor) clampSelection(sr, sc, er, ec int) (int, int, int, int) {
	if sr < 0 {
		sr = 0
	}
	if er >= len(e.lines) {
		er = len(e.lines) - 1
	}
	if sc > len([]rune(e.lines[sr])) {
		sc = len([]rune(e.lines[sr]))
	}
	if ec > len([]rune(e.lines[er])) {
		ec = len([]rune(e.lines[er]))
	}
	return sr, sc, er, ec
}

func maxFloat(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func (e *codeEditor) renderHover(size fyne.Size, lh float32) {
	lines := wrapHover(e.hoverText, 58)
	if len(lines) == 0 {
		return
	}
	textSize := e.captionSize() - 1
	if textSize < 9 {
		textSize = 9
	}
	lineH := textSize + 3
	padX, padY := float32(7), float32(4)
	width := float32(180)
	for _, line := range lines {
		if w := fyne.MeasureText(line, textSize, fyne.TextStyle{Monospace: true}).Width + padX*2; w > width {
			width = w
		}
	}
	if width > 460 {
		width = 460
	}
	if width > size.Width-24 {
		width = size.Width - 24
	}
	height := float32(len(lines))*lineH + padY*2
	x := e.textX() + measureRunes(e.lines[e.row], e.col, e.textSize())
	if x+width > size.Width-8 {
		x = size.Width - width - 8
	}
	if x < e.gutterWidth()+8 {
		x = e.gutterWidth() + 8
	}
	y := 4 + float32(e.row+1)*lh - e.scrollY + 4
	if y+height > size.Height-8 {
		y = 4 + float32(e.row)*lh - e.scrollY - height - 4
	}
	if y < 8 {
		y = 8
	}
	bg := canvas.NewRectangle(colorFloat)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	bg.CornerRadius = 3
	bg.Move(fyne.NewPos(x, y))
	bg.Resize(fyne.NewSize(width, height))
	e.surface.Add(bg)
	for i, line := range lines {
		t := canvas.NewText(line, colorText)
		t.TextSize = textSize
		t.TextStyle = fyne.TextStyle{Monospace: true}
		t.Move(fyne.NewPos(x+padX, y+padY+float32(i)*lineH))
		e.surface.Add(t)
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

func (e *codeEditor) renderCompletion(size fyne.Size, gw, lh float32) {
	if len(e.comp.items) == 0 {
		return
	}
	width := float32(330)
	if width > size.Width-24 {
		width = size.Width - 24
	}
	rowH := float32(20)
	maxRows := 10
	if avail := int((size.Height - 28) / rowH); avail < maxRows {
		maxRows = avail
	}
	if maxRows < 1 {
		maxRows = 1
	}
	rows := len(e.comp.items)
	if rows > maxRows {
		rows = maxRows
	}
	e.comp.rows = rows
	if e.comp.scroll > len(e.comp.items)-rows {
		e.comp.scroll = len(e.comp.items) - rows
	}
	if e.comp.scroll < 0 {
		e.comp.scroll = 0
	}
	height := float32(rows)*rowH + 6
	x := e.textX() + measureRunes(e.lines[e.row], e.comp.col, e.textSize())
	if x+width > size.Width-8 {
		x = size.Width - width - 8
	}
	if x < gw+8 {
		x = gw + 8
	}
	y := 4 + float32(e.row+1)*lh - e.scrollY
	if y+height > size.Height-8 {
		y = 4 + float32(e.row)*lh - e.scrollY - height - 2
	}
	if y < 8 {
		y = 8
	}
	e.comp.x, e.comp.y, e.comp.w, e.comp.h, e.comp.rowH = x, y, width, height, rowH

	bg := canvas.NewRectangle(colorFloat)
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	bg.CornerRadius = 3
	bg.Move(fyne.NewPos(x, y))
	bg.Resize(fyne.NewSize(width, height))
	e.surface.Add(bg)

	caption := e.captionSize() - 1
	if caption < 9 {
		caption = 9
	}
	for visible := 0; visible < rows; visible++ {
		idx := e.comp.scroll + visible
		if idx >= len(e.comp.items) {
			break
		}
		it := e.comp.items[idx]
		rowY := y + 3 + float32(visible)*rowH
		selected := idx == e.comp.selected
		if selected {
			sel := canvas.NewRectangle(selectionColor())
			sel.Move(fyne.NewPos(x+3, rowY))
			sel.Resize(fyne.NewSize(width-6, rowH))
			e.surface.Add(sel)
			bar := canvas.NewRectangle(colorAccent)
			bar.Move(fyne.NewPos(x+3, rowY))
			bar.Resize(fyne.NewSize(2, rowH))
			e.surface.Add(bar)
		}
		labelColor := colorText
		detailColor := colorMuted
		iconColor := colorAccent
		icon := "·"
		if strings.Contains(it.Detail, "(") {
			icon = "ƒ"
		}
		ic := canvas.NewText(icon, iconColor)
		ic.TextSize = caption
		ic.TextStyle = fyne.TextStyle{Monospace: true}
		ic.Move(fyne.NewPos(x+7, rowY+2))
		e.surface.Add(ic)

		label := canvas.NewText(oneLineText(it.Label, 36), labelColor)
		label.TextSize = caption
		label.TextStyle = fyne.TextStyle{Monospace: true}
		label.Move(fyne.NewPos(x+19, rowY+2))
		e.surface.Add(label)
		if it.Detail != "" {
			lw := fyne.MeasureText(label.Text, caption, label.TextStyle).Width
			detail := canvas.NewText(oneLineText(it.Detail, 34), detailColor)
			detail.TextSize = caption
			detail.TextStyle = fyne.TextStyle{Monospace: true}
			detail.Move(fyne.NewPos(x+19+lw+10, rowY+2))
			e.surface.Add(detail)
		}
	}
	if len(e.comp.items) > rows {
		track := canvas.NewRectangle(colorBorder)
		track.Move(fyne.NewPos(x+width-4, y+3))
		track.Resize(fyne.NewSize(2, height-6))
		e.surface.Add(track)
		thumbH := (height - 6) * float32(rows) / float32(len(e.comp.items))
		if thumbH < 12 {
			thumbH = 12
		}
		maxScroll := len(e.comp.items) - rows
		frac := float32(0)
		if maxScroll > 0 {
			frac = float32(e.comp.scroll) / float32(maxScroll)
		}
		thumb := canvas.NewRectangle(colorFaint)
		thumb.Move(fyne.NewPos(x+width-4, y+3+frac*(height-6-thumbH)))
		thumb.Resize(fyne.NewSize(2, thumbH))
		e.surface.Add(thumb)
	}
}

// selectedOccurrence reports a single-line, non-trivial selection whose other
// occurrences should be highlighted.
func (e *codeEditor) selectedOccurrence() (text string, row, sc, ec int, ok bool) {
	sr, sc, er, ec, ok := e.selection()
	if !ok || sr != er {
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
		match := true
		for j := range needle {
			if line[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			out = append(out, i)
		}
	}
	return out
}

func occurrenceColor() color.Color {
	c := colorAccent2
	c.A = 75
	return c
}

func selectionColor() color.Color {
	c := colorAccent
	c.A = 70
	return c
}

func severityColor(sev int) color.Color {
	if sev == 1 {
		return colorDanger
	}
	return colorWarning
}

type hlSpan struct {
	text  string
	color color.Color
}

// highlightLua is a deliberately small line-oriented Luau highlighter. It
// covers comments, strings (including escapes), numbers, keywords and names;
// long-bracket strings spanning lines remain body-colored.
var (
	darkKeyword  = color.NRGBA{0xC5, 0x86, 0xC0, 0xFF}
	darkString   = color.NRGBA{0xCE, 0x91, 0x78, 0xFF}
	darkNumber   = color.NRGBA{0xB5, 0xCE, 0xA8, 0xFF}
	darkComment  = color.NRGBA{0x6A, 0x99, 0x55, 0xFF}
	darkFunction = color.NRGBA{0xDC, 0xDC, 0xAA, 0xFF}
	darkBuiltin  = color.NRGBA{0x4F, 0xC1, 0xFF, 0xFF}

	lightKeyword  = color.NRGBA{0xAF, 0x00, 0xDB, 0xFF}
	lightString   = color.NRGBA{0xA3, 0x15, 0x15, 0xFF}
	lightNumber   = color.NRGBA{0x09, 0x86, 0x58, 0xFF}
	lightComment  = color.NRGBA{0x00, 0x80, 0x00, 0xFF}
	lightFunction = color.NRGBA{0x79, 0x5E, 0x26, 0xFF}
	lightBuiltin  = color.NRGBA{0x00, 0x70, 0xC1, 0xFF}
)

func synColor(dark, light color.NRGBA) color.Color {
	if activeTheme == themeLight {
		return light
	}
	return dark
}

func highlightLua(line string) []hlSpan {
	rs := []rune(line)
	var out []hlSpan
	flush := func(sb *strings.Builder, col color.Color) {
		if sb.Len() == 0 {
			return
		}
		out = append(out, hlSpan{sb.String(), col})
		sb.Reset()
	}
	plain := &strings.Builder{}
	i := 0
	for i < len(rs) {
		r := rs[i]
		if r == '-' && i+1 < len(rs) && rs[i+1] == '-' {
			flush(plain, colorText)
			out = append(out, hlSpan{string(rs[i:]), synColor(darkComment, lightComment)})
			return out
		}
		if r == '"' || r == '\'' {
			flush(plain, colorText)
			j := i + 1
			for j < len(rs) {
				if rs[j] == '\\' {
					j += 2
					continue
				}
				if rs[j] == r {
					j++
					break
				}
				j++
			}
			if j > len(rs) {
				j = len(rs)
			}
			out = append(out, hlSpan{string(rs[i:j]), synColor(darkString, lightString)})
			i = j
			continue
		}
		if unicode.IsDigit(r) {
			flush(plain, colorText)
			j := i + 1
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'x' || rs[j] == 'X' ||
				(rs[j] >= 'a' && rs[j] <= 'f') || (rs[j] >= 'A' && rs[j] <= 'F')) {
				j++
			}
			out = append(out, hlSpan{string(rs[i:j]), synColor(darkNumber, lightNumber)})
			i = j
			continue
		}
		if isIdentRune(r) {
			flush(plain, colorText)
			j := i + 1
			for j < len(rs) && isIdentRune(rs[j]) {
				j++
			}
			word := string(rs[i:j])
			var col color.Color = colorText
			switch {
			case luaKeyword(word):
				col = synColor(darkKeyword, lightKeyword)
			case preludeRoot(word):
				col = synColor(darkBuiltin, lightBuiltin)
			case j < len(rs) && rs[j] == '(':
				col = synColor(darkFunction, lightFunction)
			}
			out = append(out, hlSpan{word, col})
			i = j
			continue
		}
		plain.WriteRune(r)
		i++
	}
	flush(plain, colorText)
	return out
}

func luaKeyword(s string) bool {
	switch s {
	case "and", "break", "do", "else", "elseif", "end", "false", "for", "function",
		"if", "in", "local", "nil", "not", "or", "repeat", "return", "then", "true",
		"until", "while", "continue", "type", "export":
		return true
	}
	return false
}

func preludeRoot(s string) bool {
	switch s {
	case "session", "llm", "tools", "store", "shell", "http", "mail", "agent", "scheduler",
		"router", "runtime", "credential", "memory", "log", "json", "os", "files", "media":
		return true
	}
	return false
}

func isIdentRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
