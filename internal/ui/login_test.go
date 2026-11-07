package ui

import (
	"runtime"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// TestFramedEntryCaretIsVisible guards the bug where every framed entry had a
// zero-width caret: Fyne draws the caret SizeNameInputBorder wide, and the
// borderless override used to set that size to 0.
func TestFramedEntryCaretIsVisible(t *testing.T) {
	if w := (borderlessEntryTheme{}).Size(theme.SizeNameInputBorder); w <= 0 {
		t.Fatalf("borderless InputBorder = %v; the caret would be invisible", w)
	}

	app := test.NewApp()
	defer app.Quit()
	app.Settings().SetTheme(studioTheme{})
	win := app.NewWindow("caret")
	e := widget.NewEntry()
	e.SetText("http://127.0.0.1:9090")
	win.SetContent(controlFrame(e))
	win.Resize(fyne.NewSize(400, 80))
	win.Canvas().Focus(e)

	var widest float32
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if r, ok := o.(*canvas.Rectangle); ok && r.Visible() && r.Size().Width <= 3 && r.Size().Height > 10 {
			widest = max(widest, r.Size().Width)
		}
		if c, ok := o.(*fyne.Container); ok {
			for _, ch := range c.Objects {
				walk(ch)
			}
		}
		if w, ok := o.(fyne.Widget); ok {
			for _, ch := range test.WidgetRenderer(w).Objects() {
				walk(ch)
			}
		}
	}
	walk(e)
	if widest <= 0 {
		t.Fatal("focused framed entry draws no caret")
	}
}

func TestDescribeURL(t *testing.T) {
	cases := []struct {
		in        string
		ok        bool
		contains  string
		transport int
	}{
		{"http://127.0.0.1:9090", true, "this machine", 0},
		{"127.0.0.1:9090", true, "this machine", 0},
		{"https://agentflow.example.com", true, "HTTPS", 0},
		{"ftp://x", false, "Unknown scheme", 0},
		{"unix:///run/agentflow/admin.sock", true, "/run/agentflow/admin.sock", 1},
		{"unix://run/agentflow.sock", false, "needs a path", 1},
		{"", true, "admin.listen", 0},
	}
	for _, c := range cases {
		got, ok := describeURL(c.in)
		if ok != c.ok || !strings.Contains(got, c.contains) {
			t.Errorf("describeURL(%q) = %q, %v; want ok=%v containing %q", c.in, got, ok, c.ok, c.contains)
		}
		if tr := transportOf(c.in); tr != c.transport {
			t.Errorf("transportOf(%q) = %d, want %d", c.in, tr, c.transport)
		}
	}
	_, pipeOK := describeURL("npipe://agentflow-admin")
	if pipeOK != (runtime.GOOS == "windows") {
		t.Errorf("named pipe accepted=%v on %s", pipeOK, runtime.GOOS)
	}
}

func TestLoginFailurePointsAtTheField(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("login")
	a := New(app, win, appconfig.Config{URL: "http://127.0.0.1:9090"})
	v := &loginView{app: a}
	win.SetContent(v.build())

	v.fail(&api.Error{Status: 401, Message: "admin token rejected"})
	if !v.token.failed || v.url.failed {
		t.Fatalf("401 should flag the token only (token=%v url=%v)", v.token.failed, v.url.failed)
	}
	v.token.setHint("", false)
	v.fail(&api.Error{Status: 0, Message: "cannot reach"})
	if !v.url.failed {
		t.Fatal("an unreachable runtime should flag the URL")
	}

	v.url.entry.SetText("unix:///tmp/a.sock")
	if v.selected != 1 {
		t.Fatalf("typing a unix URL selected segment %d", v.selected)
	}
	v.selectTransport(0, true)
	if v.url.entry.Text != loginTransports[0].example {
		t.Fatalf("choosing HTTP should fill its default, got %q", v.url.entry.Text)
	}

	v.setBusy(true, "contacting…")
	if v.connectBtn.Text != "Cancel" || !v.progressRow.Visible() {
		t.Fatal("busy state should offer Cancel and show progress")
	}
	v.setBusy(false, "")
	if v.connectBtn.Text != "Connect" || v.progressRow.Visible() {
		t.Fatal("idle state should offer Connect and hide progress")
	}
}
