package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/lua"
	"agentflow-studio/internal/lua/lsp"
)

// luaEditor is the builder's loop pane: a syntax-highlighted code editor wired
// to the embedded language server. Diagnostics are drawn in the editor as
// underlines and error-lens text; completion is automatic while typing and on
// Ctrl+Space.
type luaEditor struct {
	app     *App
	editor  *codeEditor
	area    *codeArea
	root    fyne.CanvasObject
	client  *lsp.Client
	uri     string
	version int

	status    *widget.Label
	statusDot *canvas.Circle
	actions   *fyne.Container

	diags         []lsp.Diagnostic
	debounce      *time.Timer
	completion    *time.Timer
	hoverSeq      int
	completionSeq int
	closed        bool
	capsFn        func() []string
	prelude       int
	onEdit        func()
}

func newLuaEditor(app *App, title, hint, source string, prelude int, capsFn func() []string) *luaEditor {
	if prelude <= 0 {
		prelude = 1
	}
	e := &luaEditor{
		app:     app,
		uri:     "file:///agentflow/builder/loop.lua",
		version: 1,
		capsFn:  capsFn,
		prelude: prelude,
	}

	e.editor = newCodeEditor(source)
	e.editor.SetMinRowsVisible(18)
	e.editor.SetLanguage("lua")
	e.editor.onHover = e.hoverAt
	e.editor.onHoverOut = func() {
		e.hoverSeq++
		e.editor.HideHover()
	}
	e.editor.onChanged = func() {
		if e.onEdit != nil {
			e.onEdit()
		}
		e.scheduleLint()
		e.scheduleCompletion()
	}

	e.editor.onCompletion = e.requestCompletion
	e.editor.onAutoComplete = e.scheduleCompletion
	e.editor.onShortcut = e.forwardShortcut

	e.statusDot = dot(toneSuccess)
	e.status = oneLine("checking…")
	e.status.SizeName = theme.SizeNameCaptionText
	e.actions = container.NewHBox()

	statusRow := container.NewHBox(
		vCenter(dotBox(e.statusDot)),
		vCenter(widthBox(168, e.status)),
		hSpacer(4),
		vCenter(e.actions),
		vCenter(iconButton(theme.ListIcon(), "Prelude API (Ctrl+Space)", func() {
			if e.app.win != nil && e.app.win.Canvas() != nil {
				e.app.win.Canvas().Focus(e.editor)
			}
			e.editor.RequestCompletion(true)
		})),
	)
	titleRow := container.NewBorder(nil, nil, subheading(title), statusRow)
	head := container.NewVBox(titleRow, oneLine(hint), widget.NewSeparator())
	// The code area adds find/replace (Ctrl+F, Ctrl+H), go to line (Ctrl+G)
	// and the status line around the editor.
	e.area = newCodeArea(e.editor)
	body := panel(container.NewBorder(
		head,
		nil,
		nil, nil, e.area.root,
	))
	e.root = body

	e.startClient()
	return e
}

func (e *luaEditor) Text() string     { return e.editor.Text() }
func (e *luaEditor) SetText(s string) { e.editor.SetText(s) }
func (e *luaEditor) addAction(obj fyne.CanvasObject) {
	e.actions.Add(vCenter(obj))
}

func (e *luaEditor) startClient() {
	client, err := lsp.FromServer()
	if err != nil {
		e.setStatus("language service unavailable", toneDanger)
		return
	}
	e.client = client
	client.SetDiagnosticsHandler(func(uri string, version int, diags []lsp.Diagnostic) {
		if uri != e.uri {
			return
		}
		onUI(func() { e.applyDiagnostics(diags) })
	})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Initialize(ctx, e.prelude, e.capabilities()); err != nil {
			onUI(func() { e.setStatus("language service error: "+err.Error(), toneDanger) })
			return
		}
		if err := client.Open(e.uri, e.Text(), e.version); err != nil {
			onUI(func() { e.setStatus("language service error: "+err.Error(), toneDanger) })
		}
	}()
}

func (e *luaEditor) capabilities() []string {
	if e.capsFn == nil {
		return nil
	}
	return e.capsFn()
}

func (e *luaEditor) focused() bool {
	if e.app == nil || e.app.win == nil || e.app.win.Canvas() == nil {
		return false
	}
	return e.app.win.Canvas().Focused() == e.editor
}

// forwardShortcut gives the canvas its global shortcuts (Ctrl+S, navigation,
// Ctrl+Enter) after the editor has declined them.
func (e *luaEditor) forwardShortcut(s fyne.Shortcut) bool {
	cs, ok := s.(*desktop.CustomShortcut)
	if !ok || e.app == nil || e.app.win == nil {
		return false
	}
	if sc, ok := e.app.win.Canvas().(fyne.Shortcutable); ok {
		sc.TypedShortcut(cs)
		return true
	}
	return false
}

func (e *luaEditor) scheduleLint() {
	if e.closed {
		return
	}
	e.setStatus("checking…", toneNeutral)
	if e.debounce != nil {
		e.debounce.Stop()
	}
	e.debounce = time.AfterFunc(220*time.Millisecond, func() {
		var src string
		var version int
		changed := false
		// The text is read on the UI goroutine and sent from this one, so this
		// has to wait for the read. With onUI it did not: the lines below ran
		// before the callback, saw changed == false, and the edit was sent to
		// the language server only when the completion path happened to push
		// the document first.
		onUIWait(func() {
			if e.closed {
				return
			}
			e.version++
			src, version = e.Text(), e.version
			changed = true
		})
		if changed && e.client != nil {
			_ = e.client.Change(e.uri, src, version)
		}
	})
}

func (e *luaEditor) scheduleCompletion() {
	if e.closed {
		return
	}
	if e.completion != nil {
		e.completion.Stop()
	}
	e.completion = time.AfterFunc(150*time.Millisecond, func() {
		onUI(func() {
			if !e.closed {
				e.editor.RequestCompletion(false)
			}
		})
	})
}

func (e *luaEditor) SetCapabilities() {
	if e.closed || e.client == nil {
		return
	}
	_ = e.client.Configure(e.prelude, e.capabilities())
}

func (e *luaEditor) applyDiagnostics(diags []lsp.Diagnostic) {
	e.diags = diags
	conv := make([]editorDiagnostic, 0, len(diags))
	errors, warnings := 0, 0
	for _, d := range diags {
		conv = append(conv, editorDiagnostic{
			Line: d.StartLine, Col: d.StartCol, EndLine: d.EndLine, EndCol: d.EndCol,
			Severity: d.Severity, Message: d.Message,
		})
		switch d.Severity {
		case int(lua.SeverityError):
			errors++
		case int(lua.SeverityWarning):
			warnings++
		}
	}
	e.editor.SetDiagnostics(conv)
	switch {
	case errors > 0:
		text := countLabel(errors, "error", "errors")
		if warnings > 0 {
			text += " · " + countLabel(warnings, "warning", "warnings")
		}
		e.setStatus(text, toneDanger)
	case warnings > 0:
		e.setStatus(countLabel(warnings, "warning", "warnings"), toneWarning)
	default:
		e.setStatus("no issues", toneSuccess)
	}
}

func countLabel(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

func (e *luaEditor) setStatus(text string, t tone) {
	if e.status == nil {
		return
	}
	e.status.SetText(text)
	e.statusDot.FillColor = toneColor(t)
	e.statusDot.Refresh()
}

// diagContains reports whether the pointer is inside a diagnostic's range.
// Zero-length diagnostics are a single caret position, not the whole line, so
// an overlapping chunk-level error cannot claim hover everywhere on line 1.
func diagContains(d lsp.Diagnostic, row, col int) bool {
	if row < d.StartLine || row > d.EndLine {
		return false
	}
	if d.StartLine == d.EndLine {
		end := d.EndCol
		if end <= d.StartCol {
			end = d.StartCol + 1
		}
		return col >= d.StartCol && col < end
	}
	if row == d.StartLine {
		return col >= d.StartCol
	}
	if row == d.EndLine {
		return col < d.EndCol
	}
	return true
}

func (e *luaEditor) hoverAt(row, col int, overDiag bool) {
	if e.closed {
		return
	}
	var msgs []string
	seen := map[string]bool{}
	for _, d := range e.diags {
		if !overDiag {
			break
		}
		if !diagContains(d, row, col) || seen[d.Message] {
			continue
		}
		seen[d.Message] = true
		msgs = append(msgs, "• "+d.Message)
	}
	if len(msgs) > 0 {
		if len(msgs) > 3 {
			msgs = append(msgs[:3], "• …")
		}
		e.editor.ShowHover(fmt.Sprintf("Line %d\n%s", row+1, strings.Join(msgs, "\n")))
		return
	}
	if e.client == nil {
		e.editor.HideHover()
		return
	}
	e.hoverSeq++
	seq := e.hoverSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		h, err := e.client.Hover(ctx, e.uri, row, col)
		if err != nil {
			return
		}
		onUI(func() {
			if e.closed || seq != e.hoverSeq {
				return
			}
			if h == nil {
				e.editor.HideHover()
				return
			}
			e.editor.ShowHover(plainDoc(h.Markdown))
		})
	}()
}

// plainDoc renders hover Markdown as a single status line.
func plainDoc(md string) string {
	md = strings.ReplaceAll(md, "\n\n", " · ")
	md = strings.ReplaceAll(md, "\n", " ")
	md = strings.ReplaceAll(md, "**", "")
	md = strings.ReplaceAll(md, "`", "")
	md = strings.ReplaceAll(md, "*capability:*", "capability:")
	md = strings.ReplaceAll(md, "*", "")
	return strings.TrimSpace(md)
}

// requestCompletion is the editor's callback: it asks the LSP for candidates at
// the caret and lets the editor filter them by the current prefix.
func (e *luaEditor) requestCompletion(prefix string, row, start int) {
	if e.closed || e.client == nil {
		return
	}
	_, col := e.editor.CursorPosition()
	e.completionSeq++
	seq := e.completionSeq
	// Completion must see the character that triggered it. The lint debounce
	// is longer than the completion debounce, so push the current document now;
	// LSP messages are ordered, making the request below analyze this text.
	e.version++
	_ = e.client.Change(e.uri, e.Text(), e.version)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		items, err := e.client.Complete(ctx, e.uri, row, col)
		if err != nil {
			return
		}
		conv := make([]completionItem, 0, len(items))
		for _, it := range items {
			conv = append(conv, completionItem{Label: it.Label, Detail: it.Detail, Insert: it.Insert})
		}
		onUI(func() {
			if e.closed || seq != e.completionSeq {
				return
			}
			r, c := e.editor.CursorPosition()
			if r != row || c != col {
				return
			}
			e.editor.ShowCompletions(conv, prefix, row, start)
		})
	}()
}

func oneLineText(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func stripSnippet(s string) string {
	rs := []rune(s)
	var b strings.Builder
	for i := 0; i < len(rs); i++ {
		if rs[i] != '$' {
			b.WriteRune(rs[i])
			continue
		}
		if i+1 < len(rs) && rs[i+1] == '{' {
			j := i + 2
			for j < len(rs) && rs[j] != ':' && rs[j] != '}' {
				j++
			}
			if j < len(rs) && rs[j] == ':' {
				k, depth := j+1, 0
				for k < len(rs) {
					if rs[k] == '{' {
						depth++
					} else if rs[k] == '}' {
						if depth == 0 {
							break
						}
						depth--
					}
					k++
				}
				if k < len(rs) {
					b.WriteString(string(rs[j+1 : k]))
					i = k
					continue
				}
			}
			for j < len(rs) && rs[j] != '}' {
				j++
			}
			i = j
			continue
		}
		if i+1 < len(rs) && rs[i+1] >= '0' && rs[i+1] <= '9' {
			j := i + 1
			for j < len(rs) && rs[j] >= '0' && rs[j] <= '9' {
				j++
			}
			i = j - 1
			continue
		}
		b.WriteRune(rs[i])
	}
	return b.String()
}

// Close releases the language server for this pane.
func (e *luaEditor) Close() {
	if e.closed {
		return
	}
	e.closed = true
	e.editor.stopCaret()
	if e.debounce != nil {
		e.debounce.Stop()
	}
	if e.completion != nil {
		e.completion.Stop()
	}
	if e.client != nil {
		e.client.Close(e.uri)
		e.client = nil
	}
}
