package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func ctrl(k fyne.KeyName) *desktop.CustomShortcut {
	return &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierControl}
}

func chord(k fyne.KeyName, m fyne.KeyModifier) *desktop.CustomShortcut {
	return &desktop.CustomShortcut{KeyName: k, Modifier: m}
}

func wantText(t *testing.T, e *codeEditor, want string) {
	t.Helper()
	if got := e.Text(); got != want {
		t.Fatalf("text:\n%q\nwant\n%q", got, want)
	}
}

func TestMapPosThroughChanges(t *testing.T) {
	changes := []change{{from: 2, to: 4, text: "xyz"}, {from: 6, to: 6, text: "!"}}
	cases := []struct{ pos, assoc, want int }{
		{0, 1, 0},  // before every change
		{3, -1, 2}, // inside a replaced range: its start
		{3, 1, 5},  // ...or its end
		{5, 1, 6},  // after the first change: shifted by +1
		{6, -1, 7}, // at an insertion, staying before it
		{6, 1, 8},  // at an insertion, moving after it
		{9, 1, 11}, // after both
	}
	for _, c := range cases {
		if got := mapPos(changes, c.pos, c.assoc); got != c.want {
			t.Errorf("mapPos(%d, %d) = %d, want %d", c.pos, c.assoc, got, c.want)
		}
	}
}

func TestMultiCaretTypingIsOneUndoStep(t *testing.T) {
	e := newTestEditor(t, "a\nb\nc")
	e.SetCursor(0, 1)
	e.addCaretVertical(1)
	e.addCaretVertical(1)
	if len(e.allSels()) != 3 {
		t.Fatalf("carets = %d, want 3", len(e.allSels()))
	}
	e.insert('!')
	e.insert('?')
	wantText(t, e, "a!?\nb!?\nc!?")
	e.backspace()
	wantText(t, e, "a!\nb!\nc!")
	e.undo()
	wantText(t, e, "a!?\nb!?\nc!?")
	e.undo()
	wantText(t, e, "a\nb\nc")
	e.redo()
	wantText(t, e, "a!?\nb!?\nc!?")
}

func TestCtrlDAddsOccurrencesAndEditsThemTogether(t *testing.T) {
	e := newTestEditor(t, "local x = x + x2\nprint(x)")
	e.SetCursor(0, 6)
	e.TypedShortcut(ctrl(fyne.KeyD)) // selects the word x
	if got := e.selectionText(); got != "x" {
		t.Fatalf("first Ctrl+D selected %q", got)
	}
	e.TypedShortcut(ctrl(fyne.KeyD))
	e.TypedShortcut(ctrl(fyne.KeyD))
	if n := len(e.allSels()); n != 3 {
		t.Fatalf("selections = %d, want 3", n)
	}
	for _, r := range "val" {
		e.insert(r)
	}
	wantText(t, e, "local val = val + val2\nprint(x)")
}

func TestSelectAllOccurrences(t *testing.T) {
	e := newTestEditor(t, "foo bar foo\nfoo")
	e.selectWord(0, 1)
	e.TypedShortcut(chord(fyne.KeyL, fyne.KeyModifierControl|fyne.KeyModifierShift))
	if n := len(e.allSels()); n != 3 {
		t.Fatalf("selections = %d, want 3", n)
	}
	e.insert('X')
	wantText(t, e, "X bar X\nX")
}

func TestToggleCommentPerLanguage(t *testing.T) {
	e := newTestEditor(t, "local a = 1\n  local b = 2")
	e.selectAll()
	e.TypedShortcut(ctrl(fyne.KeySlash))
	wantText(t, e, "-- local a = 1\n--   local b = 2")
	e.TypedShortcut(ctrl(fyne.KeySlash))
	wantText(t, e, "local a = 1\n  local b = 2")

	y := newTestEditor(t, "a: 1\nb: 2")
	y.SetLanguage("yaml")
	y.SetCursor(1, 0)
	y.toggleComment()
	wantText(t, y, "a: 1\n# b: 2")

	m := newTestEditor(t, "hello")
	m.SetLanguage("markdown")
	m.toggleComment()
	wantText(t, m, "<!-- hello -->")
	m.toggleComment()
	wantText(t, m, "hello")
}

func TestLineCommands(t *testing.T) {
	e := newTestEditor(t, "one\ntwo\nthree")
	e.SetCursor(1, 1)
	e.TypedShortcut(chord(fyne.KeyDown, fyne.KeyModifierAlt|fyne.KeyModifierShift))
	wantText(t, e, "one\ntwo\ntwo\nthree")
	if e.row != 2 {
		t.Fatalf("copy down left the caret on row %d, want the copy (2)", e.row)
	}
	e.TypedShortcut(chord(fyne.KeyUp, fyne.KeyModifierAlt))
	e.TypedShortcut(chord(fyne.KeyUp, fyne.KeyModifierAlt))
	wantText(t, e, "two\none\ntwo\nthree")
	if e.row != 0 || e.col != 1 {
		t.Fatalf("moved caret = %d:%d, want 0:1", e.row, e.col)
	}
	e.TypedShortcut(chord(fyne.KeyK, fyne.KeyModifierControl|fyne.KeyModifierShift))
	wantText(t, e, "one\ntwo\nthree")
	e.SetCursor(0, 0)
	e.TypedShortcut(ctrl(fyne.KeyJ))
	wantText(t, e, "one two\nthree")
	e.undo()
	wantText(t, e, "one\ntwo\nthree")
}

func TestChangeCaseAndIndentShortcuts(t *testing.T) {
	e := newTestEditor(t, "hello world")
	e.SetCursor(0, 2)
	e.TypedShortcut(chord(fyne.KeyU, fyne.KeyModifierControl|fyne.KeyModifierShift))
	wantText(t, e, "HELLO world")
	e.TypedShortcut(ctrl(fyne.KeyRightBracket))
	wantText(t, e, "  HELLO world")
	e.TypedShortcut(ctrl(fyne.KeyLeftBracket))
	wantText(t, e, "HELLO world")
}

func TestBackspaceStepsBackAnIndentLevel(t *testing.T) {
	e := newTestEditor(t, "    x")
	e.SetCursor(0, 4)
	e.backspace()
	wantText(t, e, "  x")
}

func TestElectricEndOutdents(t *testing.T) {
	e := newTestEditor(t, "if x then\n  y()\n  ")
	e.SetCursor(2, 2)
	for _, r := range "end" {
		e.insert(r)
	}
	wantText(t, e, "if x then\n  y()\nend")
}

func TestMarkdownListContinues(t *testing.T) {
	e := newTestEditor(t, "- one")
	e.SetLanguage("markdown")
	e.SetCursor(0, 5)
	e.insert('\n')
	wantText(t, e, "- one\n- ")
	e.insert('\n') // an empty item ends the list
	wantText(t, e, "- one\n")
	n := newTestEditor(t, "1. a")
	n.SetLanguage("markdown")
	n.SetCursor(0, 4)
	n.insert('\n')
	wantText(t, n, "1. a\n2. ")
}

func TestNewlineBetweenBracketsOpensABlock(t *testing.T) {
	e := newTestEditor(t, "local t = {}")
	e.SetCursor(0, 11)
	e.insert('\n')
	wantText(t, e, "local t = {\n  \n}")
	if e.row != 1 || e.col != 2 {
		t.Fatalf("caret = %d:%d, want 1:2", e.row, e.col)
	}
}

func TestTypingWrapsASelection(t *testing.T) {
	e := newTestEditor(t, "value")
	e.selectAll()
	e.insert('(')
	wantText(t, e, "(value)")
	if got := e.selectionText(); got != "value" {
		t.Fatalf("selection after wrapping = %q", got)
	}
}

func TestFindReplaceAndRegex(t *testing.T) {
	e := newTestEditor(t, "foo Foo food\nfoo")
	e.setSearch(searchQuery{text: "foo"})
	if len(e.matches) != 4 {
		t.Fatalf("case-insensitive matches = %d, want 4", len(e.matches))
	}
	e.setSearch(searchQuery{text: "foo", caseSensitive: true, wholeWord: true})
	if len(e.matches) != 2 {
		t.Fatalf("whole-word matches = %d, want 2", len(e.matches))
	}
	e.SetCursor(0, 0)
	e.findNext(false)
	if got := e.selectionText(); got != "foo" || e.row != 0 {
		t.Fatalf("first match: %q at row %d", got, e.row)
	}
	e.replaceCurrent("bar")
	wantText(t, e, "bar Foo food\nfoo")
	if e.row != 1 {
		t.Fatalf("replace did not move on to the next match (row %d)", e.row)
	}
	e.setSearch(searchQuery{text: `f(o+)`, regex: true, caseSensitive: true})
	if n := e.replaceAll("F$1"); n != 2 {
		t.Fatalf("regex replace count = %d, want 2", n)
	}
	wantText(t, e, "bar Foo Food\nFoo")
	e.undo()
	wantText(t, e, "bar Foo food\nfoo")
	e.setSearch(searchQuery{text: "(", regex: true})
	if e.searchErr == nil {
		t.Fatal("a bad pattern reported no error")
	}
}

func TestSelectAllMatchesMakesCarets(t *testing.T) {
	e := newTestEditor(t, "a1 a2 a3")
	e.setSearch(searchQuery{text: `a\d`, regex: true})
	e.selectAllMatches()
	e.insert('z')
	wantText(t, e, "z z z")
}

func TestFoldingHidesLinesAndUnfoldsForTheCaret(t *testing.T) {
	src := "function loop()\n  local a = 1\n  local b = 2\nend\nreturn"
	e := newTestEditor(t, src)
	e.toggleFold(0)
	if n := len(e.layout()); n != 3 {
		t.Fatalf("visual lines with the body folded = %d, want 3", n)
	}
	e.SetCursor(0, 0)
	e.move(1, 0, false)
	if e.row != 3 {
		t.Fatalf("down from a folded line went to row %d, want 3 (end)", e.row)
	}
	e.SetCursor(2, 0) // inside the fold
	if len(e.folds) != 0 {
		t.Fatal("a caret inside a fold did not open it")
	}
	e.foldAll()
	e.unfoldAll()
	if n := len(e.layout()); n != 5 {
		t.Fatalf("visual lines after unfold all = %d, want 5", n)
	}
}

func TestFoldsFollowEditsAbove(t *testing.T) {
	e := newTestEditor(t, "x\nif a then\n  b()\nend")
	e.toggleFold(1)
	e.SetCursor(0, 1)
	e.insert('\n')
	if !e.folds[2] || e.folds[1] {
		t.Fatalf("fold did not move down with its line: %v", e.folds)
	}
}

func TestSoftWrapSplitsAtSpaces(t *testing.T) {
	e := newTestEditor(t, "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	e.width, e.height = 260, 300
	e.SetWrap(true)
	vls := e.layout()
	if len(vls) < 2 {
		t.Fatalf("a long line did not wrap: %d visual lines", len(vls))
	}
	for _, v := range vls[:len(vls)-1] {
		if r := []rune(e.lines[0]); r[v.to-1] != ' ' {
			t.Fatalf("wrapped mid-word at %d: %q", v.to, string(r[v.from:v.to]))
		}
	}
	e.SetCursor(0, 0)
	e.move(1, 0, false)
	if e.row != 0 || e.col == 0 {
		t.Fatalf("down on a wrapped line should stay in the row; got %d:%d", e.row, e.col)
	}
}

func TestColumnSelectionTypesOnEveryLine(t *testing.T) {
	e := newTestEditor(t, "abcd\nefgh\nij")
	e.columnSelect(tpos{0, 1}, tpos{2, 3})
	e.insert('_')
	wantText(t, e, "a_d\ne_h\ni_")
}

func TestMultiCaretClipboardDistributes(t *testing.T) {
	e := newTestEditor(t, "x\ny")
	e.SetCursor(0, 1)
	e.addCaretVertical(1)
	e.clipboard().SetContent("1\n2")
	e.paste()
	wantText(t, e, "x1\ny2")
}

func TestCopyWithoutSelectionTakesTheLine(t *testing.T) {
	e := newTestEditor(t, "keep\nmove")
	e.SetCursor(1, 2)
	e.cutSelection()
	wantText(t, e, "keep")
	e.SetCursor(0, 0)
	e.paste()
	wantText(t, e, "move\nkeep")
}

func TestBracketMatchingSkipsStrings(t *testing.T) {
	e := newTestEditor(t, `f(")", g(1))`)
	at, m, ok := e.matchingBracket(tpos{0, 1})
	if !ok || at.col != 1 || m.col != 11 {
		t.Fatalf("match = %v %v %v, want the last ) at 11", at, m, ok)
	}
}

func TestLuaLongCommentSpansLines(t *testing.T) {
	e := newTestEditor(t, "--[[ start\nstill comment\n]] local x")
	spans := e.spansAt(1)
	if len(spans) != 1 || spans[0].kind != 'c' {
		t.Fatalf("middle line of a long comment: %+v", spans)
	}
	last := e.spansAt(2)
	if last[0].kind != 'c' || last[len(last)-1].kind == 'c' {
		t.Fatalf("the code after ]] is still a comment: %+v", last)
	}
}

func TestYAMLBlockScalarIsAString(t *testing.T) {
	spans, state := highlightYAML("prompt: |", 0)
	if state == 0 {
		t.Fatalf("a block scalar did not change the state: %+v", spans)
	}
	body, st2 := highlightYAML("  any: thing", state)
	if body[0].kind != 's' || st2 != state {
		t.Fatalf("block body: %+v state %d", body, st2)
	}
	out, st3 := highlightYAML("next: 1", st2)
	if st3 != 0 || out[0].kind == 's' {
		t.Fatalf("dedent did not end the block: %+v", out)
	}
}

func TestHighlightersKeepEveryRune(t *testing.T) {
	samples := map[string]string{
		"lua":      `local s = [[x]] .. "y" -- z`,
		"yaml":     `- name: "a" # note`,
		"json":     `{"a": [1, true, null]}`,
		"markdown": "## Head *em* `code` [l](u)",
	}
	for lang, line := range samples {
		spans, _ := languageFor(lang).highlight(line, 0)
		joined := ""
		for _, s := range spans {
			joined += s.text
		}
		if joined != line {
			t.Errorf("%s highlighter lost text: %q", lang, joined)
		}
	}
}

func TestEscapeCollapsesCaretsBeforeLeaving(t *testing.T) {
	e := newTestEditor(t, "a\nb")
	left := false
	e.onShortcut = func(fyne.Shortcut) bool { left = true; return true }
	e.addCaretVertical(1)
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if e.multi() || left {
		t.Fatalf("Esc should first drop the extra carets (multi=%v left=%v)", e.multi(), left)
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	if !left {
		t.Fatal("a second Esc should reach the window")
	}
}

func TestCodeAreaGoToLine(t *testing.T) {
	newTestEditor(t, "")
	a := newCodeAreaText("one\ntwo\nthree", "text")
	a.openGoto()
	a.gotoLine.SetText("3:2")
	a.jump()
	if a.editor.row != 2 || a.editor.col != 1 {
		t.Fatalf("go to 3:2 landed on %d:%d", a.editor.row, a.editor.col)
	}
	if a.barSlot.Visible() {
		t.Fatal("the bar stayed open after the jump")
	}
}

func TestCodeAreaFindCountsAndCloses(t *testing.T) {
	newTestEditor(t, "")
	a := newCodeAreaText("x y x", "text")
	a.openFind(false)
	a.find.SetText("x")
	if got := a.count.Text; got != "1 of 2" {
		t.Fatalf("count = %q, want 1 of 2", got)
	}
	a.next(false)
	if got := a.count.Text; got != "2 of 2" {
		t.Fatalf("count after next = %q", got)
	}
	a.close()
	if len(a.editor.matches) != 0 {
		t.Fatal("closing the bar kept the highlights")
	}
}

// Opening the bar must lay the frame out again; before, the find row kept the
// zero width it had while hidden and the renderer panicked on its entry.
func TestFindBarRendersWhenOpened(t *testing.T) {
	newTestEditor(t, "")
	a := newCodeAreaText("x session", "lua")
	w := test.NewWindow(a.root)
	w.Resize(fyne.NewSize(900, 400))
	w.Canvas().Capture()
	a.openFind(true)
	a.find.SetText("session")
	w.Canvas().Capture()
	if a.find.Size().Width <= 0 {
		t.Fatalf("find entry width = %v", a.find.Size().Width)
	}
	a.openGoto()
	w.Canvas().Capture()
	a.close()
	w.Canvas().Capture()
}
