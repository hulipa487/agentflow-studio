package ui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
)

// ---- transports -----------------------------------------------------------------

// loginTransport is one way to reach the admin API.
type loginTransport struct {
	label       string
	example     string // the URL a fresh selection fills in
	placeholder string
	// supported is false where this OS cannot dial it; the segment is then
	// disabled with the reason as its hint.
	supported bool
	why       string
}

var loginTransports = []loginTransport{
	{label: "HTTP", example: "http://127.0.0.1:9090", placeholder: "http://127.0.0.1:9090", supported: true},
	{label: "Unix socket", example: "unix:///run/agentflow/admin.sock", placeholder: "unix:///run/agentflow/admin.sock", supported: true},
	{label: "Named pipe", example: "npipe://agentflow-admin", placeholder: "npipe://agentflow-admin",
		supported: runtime.GOOS == "windows", why: "Named pipes are Windows only"},
}

// transportOf classifies a URL as typed: 0 HTTP(S), 1 Unix socket, 2 pipe.
func transportOf(raw string) int {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "unix://"):
		return 1
	case strings.HasPrefix(raw, "npipe://"), strings.HasPrefix(raw, "pipe://"), strings.HasPrefix(raw, `\\.\pipe\`):
		return 2
	}
	return 0
}

// describeURL explains what a URL will connect to, or why it will not, before
// the operator presses Connect. ok=false is a definite mistake.
func describeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "Where the runtime's admin API listens (runtime.admin.listen, default 127.0.0.1:9090).", true
	}
	switch transportOf(raw) {
	case 1:
		path := strings.TrimPrefix(raw, "unix://")
		if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, ".") && !(len(path) > 1 && path[1] == ':') {
			return "A Unix socket URL needs a path, e.g. unix:///run/agentflow/admin.sock.", false
		}
		return "Unix socket " + path + " · runtime.admin.socket. Owner-only; the token is still required.", true
	case 2:
		if runtime.GOOS != "windows" {
			return "Named pipes are only available on Windows; use HTTP or a Unix socket here.", false
		}
		return "Named pipe on this machine · runtime.admin.socket. Owner-only; the token is still required.", true
	}
	u := raw
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "Not a URL. Use http://host:port, unix:///path or npipe://name.", false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "Unknown scheme " + parsed.Scheme + "://. Use http, https, unix or npipe.", false
	}
	host := parsed.Hostname()
	where := "a remote host: keep it on TLS or a private network"
	if host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1" {
		where = "this machine"
	}
	scheme := "HTTP"
	if parsed.Scheme == "https" {
		scheme = "HTTPS"
	}
	return fmt.Sprintf("%s to %s · %s.", scheme, parsed.Host, where), true
}

// ---- focus-aware field --------------------------------------------------------

type fieldState int

const (
	fieldIdle fieldState = iota
	fieldFocused
	fieldError
)

// loginField frames an entry with a label above and a hint below. The frame
// turns red when the field is the one to fix; focus is shown by the entry's
// own ring, which sits on the frame's outline.
type loginField struct {
	entry  *widget.Entry
	border *canvas.Rectangle
	hint   *widget.Label
	root   fyne.CanvasObject
	failed bool
}

func newLoginField(label string, entry *widget.Entry, hint string) *loginField {
	f := &loginField{entry: entry}
	f.border = canvas.NewRectangle(colorBG)
	f.border.CornerRadius = 8
	f.border.StrokeColor = colorBorder
	f.border.StrokeWidth = 1
	inner := container.NewStack(f.border,
		container.New(singleLineEntryLayout{}, container.NewThemeOverride(entry, borderlessEntryTheme{})))
	f.hint = caption(hint)
	f.hint.Wrapping = fyne.TextWrapWord
	f.root = vstack(3, compactLabel(fieldLabel(label), true), container.New(controlFrameLayout{}, inner), compactLabel(f.hint, true))
	return f
}

func (f *loginField) paint() {
	if f.failed {
		f.border.StrokeColor = colorDanger
		f.border.StrokeWidth = 1.5
		f.hint.Importance = widget.DangerImportance
	} else {
		f.border.StrokeColor = colorBorder
		f.border.StrokeWidth = 1
		f.hint.Importance = widget.LowImportance
	}
	f.border.Refresh()
	f.hint.Refresh()
}

// setHint replaces the hint; failed marks the field as the one to fix.
func (f *loginField) setHint(text string, failed bool) {
	f.hint.SetText(text)
	f.failed = failed
	f.paint()
}

// ---- the screen -------------------------------------------------------------------

type loginOptions struct {
	err  string // shown on arrival (e.g. after a failed auto-connect)
	auto bool   // reconnect with the remembered URL and token right away
}

// showLogin is the login screen with an optional error message.
func (a *App) showLogin(errMsg string) {
	a.showLoginWith(loginOptions{err: errMsg})
}

func (a *App) showLoginWith(opts loginOptions) {
	a.cancelChat()
	a.cancelView()
	a.stopHeartbeat()
	a.clearActions()
	a.view = "login"
	a.shell = nil
	a.railPane = nil
	a.railToggle = nil
	a.railVisible = false

	v := &loginView{app: a}
	a.setContent(v.build())
	a.win.Resize(fyne.NewSize(1080, 800))
	// Wrapped hints size themselves from the laid-out width; a window that
	// is already 1080x800 gets no resize pass, so lay the screen out once more.
	if c := a.win.Canvas(); c != nil && c.Content() != nil {
		c.Content().Refresh()
	}
	if opts.err != "" {
		v.fail(errors.New(opts.err))
	}
	if opts.auto {
		v.connect()
		return
	}
	v.focusFirst()
}

type loginView struct {
	app *App

	url, token  *loginField
	segments    []*widget.Button
	remember    *widget.Check
	message     *fyne.Container
	activity    *widget.Activity
	progress    *widget.Label
	progressRow *fyne.Container
	form        *fyne.Container
	connectBtn  *widget.Button
	cancel      context.CancelFunc
	selected    int
}

func (v *loginView) build() fyne.CanvasObject {
	a := v.app

	urlEntry := widget.NewEntry()
	tokenEntry := widget.NewPasswordEntry()
	tokenEntry.SetPlaceHolder("ADMIN_TOKEN or an operator token")
	v.url = newLoginField("Instance URL", urlEntry, "")
	v.token = newLoginField("Admin token", tokenEntry,
		"ADMIN_TOKEN, a named operator token (runtime.admin.operators), or the per-boot token AgentFlow prints at startup.")

	// Transport segments: choosing one fills in its default address unless
	// the field already holds an address of that kind; typing a URL selects
	// the matching segment.
	seg := hstack(spaceXS)
	for i, t := range loginTransports {
		i, t := i, t
		b := widget.NewButton(t.label, func() { v.selectTransport(i, true) })
		if !t.supported {
			b.Disable()
		}
		v.segments = append(v.segments, b)
		seg.Add(b)
	}

	initial := a.cfg.URL
	if initial == "" {
		initial = loginTransports[0].example
	}
	urlEntry.SetText(initial)
	urlEntry.CursorColumn = len([]rune(initial))
	urlEntry.OnChanged = func(s string) {
		v.highlight(transportOf(s))
		v.refreshURLHint()
		v.clearMessage()
	}
	if a.cfg.Token != "" {
		tokenEntry.SetText(a.cfg.Token)
		tokenEntry.CursorColumn = len([]rune(a.cfg.Token))
	}
	tokenEntry.OnChanged = func(string) {
		if v.token.failed {
			v.token.setHint("ADMIN_TOKEN, a named operator token (runtime.admin.operators), or the per-boot token AgentFlow prints at startup.", false)
		}
		v.clearMessage()
	}
	urlEntry.OnSubmitted = func(string) { a.win.Canvas().Focus(tokenEntry) }
	tokenEntry.OnSubmitted = func(string) { v.connect() }
	v.highlight(transportOf(initial))
	v.refreshURLHint()

	cfgPath, _ := appconfig.Path()
	v.remember = widget.NewCheck("Remember the token on this computer", nil)
	v.remember.SetChecked(!a.cfg.ForgetToken)
	rememberNote := caption("Stored in plain text, owner-only, in " + orDash(cfgPath) + ". Lock forgets it.")
	rememberNote.Wrapping = fyne.TextWrapWord

	v.message = vstack(spaceXS)
	v.activity = widget.NewActivity()
	v.progress = caption("")
	v.progress.Wrapping = fyne.TextWrapOff
	v.progressRow = container.NewCenter(hstack(spaceXS, v.activity, v.progress))
	v.progressRow.Hide()
	v.connectBtn = primaryButton("Connect", theme.NavigateNextIcon(), v.connect)

	cardTitle := titleText("Connect to a runtime", theme.SizeNameSubHeadingText)
	v.form = vstack(spaceM,
		vstack(2, insetLeading(cardTitle), compactLabel(muted("Studio talks to AgentFlow's admin API with its bearer token. No browser, no CORS."), true)),
		vstack(spaceXS, fieldLabel("Transport"), seg),
		v.url.root,
		v.token.root,
		vstack(0, v.remember, insetLeading(rememberNote)),
		v.message,
		controlFrame(v.connectBtn),
		v.progressRow,
	)
	card := panel(container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceS, spaceS), widthBox(440, v.form)))

	title := widget.NewLabelWithStyle("AgentFlow Studio", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	title.SizeName = theme.SizeNameHeadingText
	tagline := muted("Build, run and trace agents on your own runtime.")
	tagline.Alignment = fyne.TextAlignCenter
	tagline.Wrapping = fyne.TextWrapOff

	themeLabel := "Light mode"
	if a.variant == themeLight {
		themeLabel = "Dark mode"
	}
	var themeBtn *widget.Button
	themeBtn = widget.NewButtonWithIcon(themeLabel, theme.ColorPaletteIcon(), func() {
		if a.variant == themeDark {
			a.chooseTheme(themeLight)
			themeBtn.SetText("Dark mode")
		} else {
			a.chooseTheme(themeDark)
			themeBtn.SetText("Light mode")
		}
	})
	themeBtn.Importance = widget.LowImportance
	version := caption("Studio " + appVersion + " ·")
	version.Wrapping = fyne.TextWrapOff
	footer := container.NewCenter(hstack(spaceXS, vCenter(version), themeBtn))

	body := vstack(spaceS,
		container.NewCenter(logoMark(72)),
		container.NewCenter(title),
		container.NewCenter(tagline),
		container.NewCenter(newTransStripe(168, 3)),
		vSpacer(spaceS),
		container.NewCenter(card),
		footer,
	)

	var bottom fyne.CanvasObject
	if grip := a.resizeGrip(); grip != nil {
		bottom = container.NewHBox(layout.NewSpacer(), grip)
	}
	return container.NewStack(
		canvas.NewRectangle(colorBG),
		container.NewBorder(a.chromeStrip(), bottom, nil, nil,
			container.NewCenter(container.NewPadded(body))),
	)
}

func (v *loginView) focusFirst() {
	c := v.app.win.Canvas()
	if c == nil {
		return
	}
	if v.app.cfg.URL == "" || strings.TrimSpace(v.token.entry.Text) != "" {
		c.Focus(v.url.entry)
		return
	}
	c.Focus(v.token.entry)
}

func (v *loginView) highlight(i int) {
	v.selected = i
	for j, b := range v.segments {
		if j == i {
			b.Importance = widget.HighImportance
		} else {
			b.Importance = widget.MediumImportance
		}
		b.Refresh()
	}
}

func (v *loginView) selectTransport(i int, fill bool) {
	v.highlight(i)
	e := v.url.entry
	if fill && transportOf(e.Text) != i {
		e.SetText(loginTransports[i].example)
		e.CursorColumn = len([]rune(e.Text))
	}
	e.SetPlaceHolder(loginTransports[i].placeholder)
	v.app.win.Canvas().Focus(e)
}

func (v *loginView) refreshURLHint() {
	text, ok := describeURL(v.url.entry.Text)
	v.url.setHint(text, !ok)
}

func (v *loginView) clearMessage() {
	if len(v.message.Objects) == 0 {
		return
	}
	clearBox(v.message)
	v.message.Refresh()
	v.form.Refresh()
}

func (v *loginView) showMessage(text string, t tone) {
	clearBox(v.message)
	if text != "" {
		v.message.Add(messageCard(text, t))
	}
	v.message.Refresh()
	v.form.Refresh()
	if c := v.app.win.Canvas(); c != nil && c.Content() != nil {
		c.Content().Refresh()
	}
}

// fail explains a failed connection and points at the field to fix.
func (v *loginView) fail(err error) {
	var ae *api.Error
	msg := err.Error()
	switch {
	case errors.As(err, &ae) && ae.Status == 401:
		v.token.setHint("The runtime rejected this token. Check ADMIN_TOKEN, or copy the token AgentFlow printed at boot (it changes every start unless ADMIN_TOKEN is set).", true)
		msg = "Authentication failed: " + ae.Message
		v.app.win.Canvas().Focus(v.token.entry)
	case errors.As(err, &ae) && ae.Status == 0:
		v.url.setHint("Nothing answered here. Is AgentFlow running, and is this its admin address (runtime.admin.listen, or runtime.admin.socket with tcp: false)?", true)
		v.app.win.Canvas().Focus(v.url.entry)
	case errors.As(err, &ae) && ae.Status == 503:
		msg += "\n\nThe Studio API is off on this runtime: set runtime.studio.enabled: true and restart AgentFlow."
	}
	v.showMessage(msg, toneDanger)
}

func (v *loginView) setBusy(busy bool, step string) {
	if busy {
		v.connectBtn.SetText("Cancel")
		v.connectBtn.SetIcon(theme.CancelIcon())
		v.connectBtn.Importance = widget.MediumImportance
		v.progressRow.Show()
		v.activity.Start()
	} else {
		v.connectBtn.SetText("Connect")
		v.connectBtn.SetIcon(theme.NavigateNextIcon())
		v.connectBtn.Importance = widget.HighImportance
		v.activity.Stop()
		v.progressRow.Hide()
	}
	v.connectBtn.Refresh()
	v.progress.SetText(step)
	v.form.Refresh()
	for _, b := range v.segments {
		if busy {
			b.Disable()
		}
	}
	if !busy {
		for i, b := range v.segments {
			if loginTransports[i].supported {
				b.Enable()
			}
		}
	}
}

// connect checks the token and the Studio API, or cancels a check in
// progress. Credentials are saved only after both succeed, so a typo never
// becomes a sticky auto-connect.
func (v *loginView) connect() {
	if v.cancel != nil {
		v.cancel()
		return
	}
	rawURL := strings.TrimSpace(v.url.entry.Text)
	token := strings.TrimSpace(v.token.entry.Text)
	if rawURL == "" {
		v.url.setHint("An instance URL is required.", true)
		v.app.win.Canvas().Focus(v.url.entry)
		return
	}
	if hint, ok := describeURL(rawURL); !ok {
		v.url.setHint(hint, true)
		v.app.win.Canvas().Focus(v.url.entry)
		return
	}
	if token == "" {
		v.token.setHint("The admin token is required.", true)
		v.app.win.Canvas().Focus(v.token.entry)
		return
	}
	remember := v.remember.Checked
	v.clearMessage()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	v.cancel = cancel
	v.setBusy(true, "contacting "+rawURL+"…")

	a := v.app
	go func() {
		defer cancel()
		c := api.New(rawURL, token)
		step := func(s string) { fyne.Do(func() { v.progress.SetText(s) }) }
		done := func(err error) {
			fyne.Do(func() {
				v.cancel = nil
				v.setBusy(false, "")
				switch {
				case errors.Is(err, context.Canceled):
					v.showMessage("Connection cancelled.", toneNeutral)
				case errors.Is(err, context.DeadlineExceeded):
					v.url.setHint("No answer within 20 seconds. Check the address and that nothing blocks the port.", true)
					v.showMessage("Timed out contacting "+rawURL+".", toneDanger)
				default:
					v.fail(err)
				}
			})
		}
		if err := c.Ping(ctx); err != nil {
			done(err)
			return
		}
		step("token accepted · loading the Studio catalog…")
		cat, err := c.Catalog(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				err = fmt.Errorf("connected, but the Studio API is unavailable: %w", err)
			}
			done(err)
			return
		}
		fyne.Do(func() {
			v.cancel = nil
			a.api = c
			a.catalog = cat
			a.cfg.URL = c.DisplayURL
			a.cfg.ForgetToken = !remember
			a.cfg.Token = ""
			if remember {
				a.cfg.Token = token
			}
			_ = appconfig.Save(a.cfg)
			a.showMain()
			a.startHeartbeat()
		})
	}()
}
