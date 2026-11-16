package ui

import (
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/appconfig"
)

// TestLoopEditorLintsAfterAnEdit: an edit reaches the language server through
// the lint debounce on its own. It used to get there only when the completion
// path happened to push the document first: the debounce asked the UI
// goroutine for the text without waiting for the answer, read "nothing
// changed", and sent nothing. The headless test driver hid that by running the
// request inline; here the completion timer is stopped so the lint path is the
// only one that can deliver the edit.
func TestLoopEditorLintsAfterAnEdit(t *testing.T) {
	fyneApp := test.NewApp()
	ui := claimUI(t)
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("editor")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})

	const clean = "-- af-prelude-version: 1\nfunction loop()\n  local m = session.inbox()\n  session.send(m.text)\nend\n"
	e := newLuaEditor(a, "loop", "", clean, 1, func() []string { return []string{"llm.chat"} })
	defer e.Close()
	win.SetContent(e.editor)

	// The document as opened is clean.
	ui.settle(300 * time.Millisecond)
	if len(e.diags) != 0 {
		t.Fatalf("the clean loop has diagnostics: %+v", e.diags)
	}

	// An edit that introduces an unknown name.
	e.SetText("-- af-prelude-version: 1\nfunction loop()\n  local m = session.inbox()\n  sesion.send(m.text)\nend\n")
	e.scheduleLint()
	if e.completion != nil {
		e.completion.Stop()
	}
	if !ui.waitFor(3*time.Second, func() bool { return len(e.diags) > 0 }) {
		t.Fatal("the edit never reached the language server: no diagnostics after the lint debounce")
	}
}
