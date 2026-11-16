package ui

import (
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/lua/lsp"
)

func newTestEditor(t *testing.T, source string) *codeEditor {
	t.Helper()
	app := test.NewApp()
	claimUI(t)
	t.Cleanup(app.Quit)
	return newCodeEditor(source)
}

func TestEditorAutoIndentAndUndo(t *testing.T) {
	e := newTestEditor(t, "function loop()\nend")
	e.SetCursor(0, len([]rune("function loop()")))
	e.insert('\n')
	if got, want := e.Text(), "function loop()\n  \nend"; got != want {
		t.Fatalf("auto-indent:\n%q\nwant\n%q", got, want)
	}
	e.undo()
	if got, want := e.Text(), "function loop()\nend"; got != want {
		t.Fatalf("undo:\n%q\nwant\n%q", got, want)
	}
}

func TestEditorAutoPairAndSelection(t *testing.T) {
	e := newTestEditor(t, "local x = \n")
	e.SetCursor(0, len([]rune("local x = ")))
	e.insert('(')
	if got := e.Text(); got != "local x = ()\n" {
		t.Fatalf("auto-pair: %q", got)
	}
	e.insert(')') // steps over the closer, does not duplicate it
	if got := e.Text(); got != "local x = ()\n" {
		t.Fatalf("pair step-over: %q", got)
	}
	e.insert('"')
	if got := e.Text(); got != "local x = ()\"\"\n" {
		t.Fatalf("quote pair: %q", got)
	}
}

func TestEditorCompletionReplacesPrefix(t *testing.T) {
	e := newTestEditor(t, "function loop()\n  ll\nend")
	e.SetCursor(1, 4)
	e.onCompletion = func(prefix string, row, start int) {
		e.ShowCompletions([]completionItem{
			{Label: "llm.chat", Detail: "model call", Insert: "llm.chat"},
			{Label: "log.info", Detail: "log line", Insert: "log.info"},
		}, prefix, row, start)
	}
	e.RequestCompletion(false)
	if !e.comp.visible {
		t.Fatal("completion did not open")
	}
	e.applyCompletion()
	if got, want := e.Text(), "function loop()\n  llm.chat\nend"; got != want {
		t.Fatalf("completion insert:\n%q\nwant\n%q", got, want)
	}
}

func TestEditorSetTextNotifies(t *testing.T) {
	e := newTestEditor(t, "")
	calls := 0
	e.onChanged = func() { calls++ }
	e.SetText("function loop() end")
	if calls != 1 {
		t.Fatalf("onChanged calls = %d, want 1", calls)
	}
	if !strings.Contains(e.Text(), "function loop()") {
		t.Fatalf("text = %q", e.Text())
	}
}

func TestHighlightLuaCoversCommentsAndStrings(t *testing.T) {
	spans := highlightLua(`local s = "hi" -- note`)
	if len(spans) < 4 {
		t.Fatalf("spans = %+v", spans)
	}
	joined := ""
	for _, s := range spans {
		joined += s.text
	}
	if joined != `local s = "hi" -- note` {
		t.Fatalf("highlight lost text: %q", joined)
	}
}

func TestEditorAutoCompletionAfterDot(t *testing.T) {
	e := newTestEditor(t, "session.")
	e.SetCursor(0, len([]rune("session.")))
	called := false
	e.onCompletion = func(prefix string, row, start int) {
		called = true
		e.ShowCompletions([]completionItem{{Label: "exit", Insert: "exit()", Detail: "session.exit()"}}, prefix, row, start)
	}
	e.RequestCompletion(false)
	if !called || !e.comp.visible {
		t.Fatalf("member completion did not open (called=%v visible=%v)", called, e.comp.visible)
	}
	e.applyCompletion()
	if got, want := e.Text(), "session.exit()"; got != want {
		t.Fatalf("member completion = %q, want %q", got, want)
	}
}

func TestEditorAutoCompleteCallbackOnTyping(t *testing.T) {
	e := newTestEditor(t, "")
	calls := 0
	e.onAutoComplete = func() { calls++ }
	e.insert('s')
	e.insert('.')
	if calls != 2 {
		t.Fatalf("auto-complete callbacks = %d, want 2", calls)
	}
}

func TestEditorReturnKeysCreateLines(t *testing.T) {
	for _, key := range []fyne.KeyName{fyne.KeyReturn, fyne.KeyEnter} {
		e := newTestEditor(t, "function loop() end")
		e.SetCursor(0, len([]rune("function loop() end")))
		e.TypedKey(&fyne.KeyEvent{Name: key})
		if got := e.Text(); got != "function loop() end\n" {
			t.Fatalf("%s did not create a line: %q", key, got)
		}
	}
}

func TestEditorReturnAcceptsCompletion(t *testing.T) {
	e := newTestEditor(t, "function loop()\n  ll\nend")
	e.SetCursor(1, 4)
	e.onCompletion = func(prefix string, row, start int) {
		e.ShowCompletions([]completionItem{{Label: "llm.chat", Insert: "llm.chat"}}, prefix, row, start)
	}
	e.RequestCompletion(false)
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyReturn})
	if got, want := e.Text(), "function loop()\n  llm.chat\nend"; got != want {
		t.Fatalf("Return did not accept completion:\n%q\nwant\n%q", got, want)
	}
}

func TestEditorDoubleClickSelectsWord(t *testing.T) {
	e := newTestEditor(t, "local value = session.inbox()")
	e.selectWord(0, 8) // inside "value"
	if got, want := e.selectionText(), "value"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	if r, c := e.CursorPosition(); r != 0 || c != 11 {
		t.Fatalf("cursor = %d:%d, want 0:11", r, c)
	}
}

func TestEditorDoubleClickSelectsOnePunctuationRune(t *testing.T) {
	e := newTestEditor(t, "local value = session.inbox()")
	e.selectWord(0, 21) // the dot before inbox
	if got := e.selectionText(); got != "." {
		t.Fatalf("selected %q, want exactly the punctuation rune", got)
	}
}

func TestEditorDoubleClickSelectsWhitespaceRun(t *testing.T) {
	e := newTestEditor(t, "local  value")
	e.selectWord(0, 5)
	if got := e.selectionText(); got != "  " {
		t.Fatalf("selected %q, want the two-space run", got)
	}
	if r, c := e.CursorPosition(); r != 0 || c != 7 {
		t.Fatalf("caret = %d:%d, want 0:7", r, c)
	}
}

func TestEditorDoubleClickPastLineEndSelectsLine(t *testing.T) {
	e := newTestEditor(t, "local value = 3\nreturn value")
	x := e.textX() + e.charWidth()*20
	e.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(x, 4)})
	if got, want := e.selectionText(), "local value = 3\n"; got != want {
		t.Fatalf("selected %q, want the whole first line %q", got, want)
	}
}

func TestEditorDoubleClickPastLastLineEndSelectsLineText(t *testing.T) {
	e := newTestEditor(t, "local value = 3\nreturn value")
	x := e.textX() + e.charWidth()*20
	y := 4 + e.lineHeight() + 1
	e.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(x, y)})
	if got, want := e.selectionText(), "return value"; got != want {
		t.Fatalf("selected %q, want the whole last line %q", got, want)
	}
	if r, c := e.CursorPosition(); r != 1 || c != len([]rune("return value")) {
		t.Fatalf("caret = %d:%d, want 1:%d", r, c, len([]rune("return value")))
	}
}

func TestEditorDoubleClickBlankLineSelectsLine(t *testing.T) {
	e := newTestEditor(t, "local x = 1\n\nreturn x")
	x := e.textX() + e.charWidth()*5
	y := 4 + e.lineHeight() + 1
	e.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(x, y)})
	if got, want := e.selectionText(), "\n"; got != want {
		t.Fatalf("selected %q, want the blank line's break %q", got, want)
	}
	if r, c := e.CursorPosition(); r != 2 || c != 0 {
		t.Fatalf("caret = %d:%d, want 2:0", r, c)
	}
}

func TestEditorDoubleClickOnLastGlyphStillSelectsWord(t *testing.T) {
	e := newTestEditor(t, "local value")
	x := e.textX() + measureRunes("local value", len([]rune("local value")), e.textSize()) - e.charWidth()/5
	e.DoubleTapped(&fyne.PointEvent{Position: fyne.NewPos(x, 4)})
	if got := e.selectionText(); got != "value" {
		t.Fatalf("selected %q, want the word under the pointer", got)
	}
}

func TestEditorClipboardShortcuts(t *testing.T) {
	e := newTestEditor(t, "alpha beta")
	e.TypedShortcut(&fyne.ShortcutSelectAll{})
	if got := e.selectionText(); got != "alpha beta" {
		t.Fatalf("select-all = %q", got)
	}
	e.TypedShortcut(&fyne.ShortcutCopy{})
	e.TypedShortcut(&fyne.ShortcutCut{})
	if e.Text() != "" {
		t.Fatalf("cut left %q", e.Text())
	}
	e.TypedShortcut(&fyne.ShortcutPaste{})
	if got := e.Text(); got != "alpha beta" {
		t.Fatalf("paste = %q", got)
	}
}

func TestEditorUsesTextCursor(t *testing.T) {
	e := newTestEditor(t, "")
	if got := e.Cursor(); got != desktop.TextCursor {
		t.Fatalf("cursor = %v, want text cursor", got)
	}
}

func TestEditorHoverIsInlineState(t *testing.T) {
	e := newTestEditor(t, "session")
	e.ShowHover("session.send(text) — reply")
	if !e.hoverVisible {
		t.Fatal("hover box did not open")
	}
	e.HideHover()
	if e.hoverVisible {
		t.Fatal("hover box did not close")
	}
}

func TestDiagnosticHoverOnlyOverSquiggle(t *testing.T) {
	e := newTestEditor(t, "abcd efgh")
	le := &luaEditor{editor: e, diags: []lsp.Diagnostic{{
		StartLine: 0, StartCol: 0, EndLine: 0, EndCol: 4,
		Severity: 1, Message: "bad token",
	}}}
	le.hoverAt(0, 2, true)
	if !e.hoverVisible {
		t.Fatal("hovering the squiggle did not show its message")
	}
	le.hoverAt(0, 8, false)
	if e.hoverVisible {
		t.Fatal("hovering outside the squiggle still showed the error")
	}
}

func TestEditorShortContentCannotScroll(t *testing.T) {
	e := newTestEditor(t, "function loop() end")
	e.width, e.height = 700, 500
	e.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -8)})
	if e.scrollY != 0 {
		t.Fatalf("short document scrolled to %v, want 0", e.scrollY)
	}
	e.scrollY = 900
	e.render(fyne.NewSize(700, 500))
	if e.scrollY != 0 {
		t.Fatalf("render kept an out-of-range scroll offset: %v", e.scrollY)
	}
}

func TestEditorCtrlBackspaceDeletesWord(t *testing.T) {
	e := newTestEditor(t, "local value")
	e.SetCursor(0, len([]rune("local value")))
	ctrl := &desktop.CustomShortcut{KeyName: fyne.KeyBackspace, Modifier: fyne.KeyModifierControl}
	e.TypedShortcut(ctrl)
	if got := e.Text(); got != "local " {
		t.Fatalf("first ctrl+backspace = %q, want %q", got, "local ")
	}
	e.TypedShortcut(ctrl)
	if got := e.Text(); got != "" {
		t.Fatalf("second ctrl+backspace = %q, want empty", got)
	}
}

func TestEditorCtrlDeleteDeletesWord(t *testing.T) {
	e := newTestEditor(t, "local value")
	e.SetCursor(0, 0)
	e.TypedShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDelete, Modifier: fyne.KeyModifierControl})
	if got := e.Text(); got != "value" {
		t.Fatalf("ctrl+delete = %q, want %q", got, "value")
	}
}

func TestEditorShiftArrowSelection(t *testing.T) {
	e := newTestEditor(t, "abc")
	e.SetCursor(0, 3)
	e.KeyDown(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if got := e.selectionText(); got != "c" {
		t.Fatalf("shift+left selected %q, want %q", got, "c")
	}
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyRight})
	e.KeyUp(&fyne.KeyEvent{Name: desktop.KeyShiftLeft})
	e.TypedKey(&fyne.KeyEvent{Name: fyne.KeyLeft})
	if e.sel {
		t.Fatalf("selection survived shift release: %q", e.selectionText())
	}
}

func TestFindOccurrences(t *testing.T) {
	got := findOccurrences([]rune("foo foo food"), []rune("foo"))
	want := []int{0, 4, 8}
	if len(got) != len(want) {
		t.Fatalf("occurrences = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("occurrences = %v, want %v", got, want)
		}
	}
}
