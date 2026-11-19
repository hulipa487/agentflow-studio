package ui

import (
	"context"
	"errors"
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
	labelKey    string
	example     string // the URL a fresh selection fills in
	placeholder string
	// supported is false where this OS cannot dial it; the segment is then
	// disabled with the reason as its hint.
	supported bool
	whyKey    string
}

var loginTransports = []loginTransport{
	{labelKey: "login.transportHttp", example: "http://127.0.0.1:8080", placeholder: "http://127.0.0.1:8080", supported: true},
	{labelKey: "login.transportUnix", example: "unix:///run/agentflow/admin.sock", placeholder: "unix:///run/agentflow/admin.sock", supported: true},
	{labelKey: "login.transportPipe", example: "npipe://agentflow-admin", placeholder: "npipe://agentflow-admin",
		supported: runtime.GOOS == "windows", whyKey: "login.pipesWindowsOnly"},
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
		return T("connection.describeEmpty"), true
	}
	switch transportOf(raw) {
	case 1:
		path := strings.TrimPrefix(raw, "unix://")
		if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, ".") && !(len(path) > 1 && path[1] == ':') {
			return T("connection.describeUnixNoPath"), false
		}
		return T("connection.describeUnix", "path", path), true
	case 2:
		if runtime.GOOS != "windows" {
			return T("connection.describePipeWindowsOnly"), false
		}
		return T("connection.describePipe"), true
	}
	u := raw
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return T("connection.describeNotURL"), false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return T("connection.describeBadScheme", "scheme", parsed.Scheme), false
	}
	host := parsed.Hostname()
	where := "remote"
	if host == "localhost" || strings.HasPrefix(host, "127.") || host == "::1" {
		where = "local"
	}
	scheme := "HTTP"
	if parsed.Scheme == "https" {
		scheme = "HTTPS"
	}
	return T("connection.describeHttp", "scheme", scheme, "host", parsed.Host, "where", where), true
}

// loginCauseError is a translated message that keeps its cause for errors.As, so
// a failure wrapped with context still points at the field to fix.
type loginCauseError struct {
	msg   string
	cause error
}

func (e loginCauseError) Error() string { return e.msg }
func (e loginCauseError) Unwrap() error { return e.cause }

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
	// A language change builds the screen again in the new language, keeping
	// what was typed; it never connects on its own. buildShell clears this.
	cur := v
	a.relocalizeView = func() {
		if cur.cancel != nil {
			cur.cancel()
		}
		next := &loginView{app: a, keep: &loginInput{
			url: cur.url.entry.Text, token: cur.token.entry.Text, remember: cur.remember.Checked,
		}}
		a.setContent(next.build())
		if c := a.win.Canvas(); c != nil && c.Content() != nil {
			c.Content().Refresh()
		}
		next.focusFirst()
		cur = next
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

// loginInput is what the operator had typed, carried over when the screen is
// built again (after a language change).
type loginInput struct {
	url, token string
	remember   bool
}

type loginView struct {
	app *App
	// keep, when set, fills the form instead of the saved configuration.
	keep *loginInput

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
	tokenEntry.SetPlaceHolder(T("login.tokenPlaceholder"))
	v.url = newLoginField(T("login.instanceURL"), urlEntry, "")
	v.token = newLoginField(T("login.adminToken"), tokenEntry, T("login.tokenHint"))

	// Transport segments: choosing one fills in its default address unless
	// the field already holds an address of that kind; typing a URL selects
	// the matching segment.
	seg := hstack(spaceXS)
	for i, t := range loginTransports {
		i, t := i, t
		b := widget.NewButton(T(t.labelKey), func() { v.selectTransport(i, true) })
		if !t.supported {
			b.Disable()
		}
		v.segments = append(v.segments, b)
		seg.Add(b)
	}

	initial := a.cfg.URL
	if v.keep != nil {
		initial = v.keep.url
	}
	if initial == "" && v.keep == nil {
		initial = loginTransports[0].example
	}
	urlEntry.SetText(initial)
	urlEntry.CursorColumn = len([]rune(initial))
	urlEntry.OnChanged = func(s string) {
		v.highlight(transportOf(s))
		v.refreshURLHint()
		v.clearMessage()
	}
	initialToken := a.cfg.Token
	if v.keep != nil {
		initialToken = v.keep.token
	}
	if initialToken != "" {
		tokenEntry.SetText(initialToken)
		tokenEntry.CursorColumn = len([]rune(initialToken))
	}
	tokenEntry.OnChanged = func(string) {
		if v.token.failed {
			v.token.setHint(T("login.tokenHint"), false)
		}
		v.clearMessage()
	}
	urlEntry.OnSubmitted = func(string) { a.win.Canvas().Focus(tokenEntry) }
	tokenEntry.OnSubmitted = func(string) { v.connect() }
	v.highlight(transportOf(initial))
	v.refreshURLHint()

	cfgPath, _ := appconfig.Path()
	v.remember = widget.NewCheck(T("login.rememberDesktop"), nil)
	if v.keep != nil {
		v.remember.SetChecked(v.keep.remember)
	} else {
		v.remember.SetChecked(!a.cfg.ForgetToken)
	}
	rememberNote := caption(T("login.rememberDesktopNote", "path", orDash(cfgPath)))
	rememberNote.Wrapping = fyne.TextWrapWord

	v.message = vstack(spaceXS)
	v.activity = widget.NewActivity()
	v.progress = caption("")
	v.progress.Wrapping = fyne.TextWrapOff
	v.progressRow = container.NewCenter(hstack(spaceXS, v.activity, v.progress))
	v.progressRow.Hide()
	v.connectBtn = primaryButton(T("login.connect"), theme.NavigateNextIcon(), v.connect)

	cardTitle := titleText(T("login.connectTitle"), theme.SizeNameSubHeadingText)
	// Saved connections are a shortlist, not a replacement for the form: a
	// click fills the URL and token, and editing either is still allowed.
	saved := vstack(spaceXS)
	if len(a.cfg.Profiles) > 0 {
		saved.Add(fieldLabel(T("login.savedConnections")))
		for _, p := range a.cfg.Profiles {
			p := p
			saved.Add(widget.NewButton(p.Name+"  "+p.URL, func() {
				urlEntry.SetText(p.URL)
				tokenEntry.SetText(p.Token)
			}))
		}
	} else {
		saved.Hide()
	}
	v.form = vstack(spaceM,
		vstack(2, insetLeading(cardTitle), compactLabel(muted(T("login.introDesktop")), true)),
		saved,
		vstack(spaceXS, fieldLabel(T("login.transport")), seg),
		v.url.root,
		v.token.root,
		vstack(0, v.remember, insetLeading(rememberNote)),
		v.message,
		controlFrame(v.connectBtn),
		v.progressRow,
	)
	card := panel(container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceS, spaceS), widthBox(440, v.form)))

	// The banner carries the AgentFlow name; this line names the app.
	title := widget.NewLabelWithStyle("Studio", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	title.SizeName = theme.SizeNameHeadingText
	tagline := muted(T("login.tagline"))
	tagline.Alignment = fyne.TextAlignCenter
	tagline.Wrapping = fyne.TextWrapOff

	themeLabel := T("login.lightMode")
	if a.variant == themeLight {
		themeLabel = T("login.darkMode")
	}
	var themeBtn *widget.Button
	themeBtn = widget.NewButtonWithIcon(themeLabel, theme.ColorPaletteIcon(), func() {
		if a.variant == themeDark {
			a.chooseTheme(themeLight)
			themeBtn.SetText(T("login.darkMode"))
		} else {
			a.chooseTheme(themeDark)
			themeBtn.SetText(T("login.lightMode"))
		}
	})
	themeBtn.Importance = widget.LowImportance
	version := caption(T("login.version", "version", appVersion))
	version.Wrapping = fyne.TextWrapOff
	footer := container.NewCenter(hstack(spaceXS, vCenter(version), themeBtn))

	body := vstack(spaceS,
		container.NewCenter(brandBanner(60, 34)),
		container.NewCenter(title),
		container.NewCenter(tagline),
		container.NewCenter(newBrandRule(168, 3)),
		vSpacer(spaceS),
		container.NewCenter(card),
		footer,
	)

	var bottom fyne.CanvasObject
	if grip := a.resizeGrip(); grip != nil {
		bottom = container.NewHBox(layout.NewSpacer(), grip)
	}
	strip := a.chromeStrip()
	// The language switch floats in the top-right corner, below the window
	// buttons when there are any, so the centered form does not move.
	corner := container.NewHBox(layout.NewSpacer(),
		container.New(layout.NewCustomPaddedLayout(spaceS, 0, 0, spaceM), a.languageToggle()))
	var cornerTop fyne.CanvasObject = corner
	if strip != nil {
		cornerTop = vstack(0, vSpacer(strip.MinSize().Height), corner)
	}
	return container.NewStack(
		canvas.NewRectangle(colorBG),
		container.NewBorder(strip, bottom, nil, nil,
			container.NewCenter(container.NewPadded(body))),
		container.NewBorder(cornerTop, nil, nil, nil),
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
		v.token.setHint(T("login.tokenRejected"), true)
		msg = T("login.authFailed", "message", ae.Message)
		v.app.win.Canvas().Focus(v.token.entry)
	case errors.As(err, &ae) && ae.Status == 0:
		v.url.setHint(T("login.nothingAnswered"), true)
		v.app.win.Canvas().Focus(v.url.entry)
	case errors.As(err, &ae) && ae.Status == 403:
		v.url.setHint(T("login.localOnly"), true)
		v.app.win.Canvas().Focus(v.url.entry)
	case errors.As(err, &ae) && ae.Status == 503:
		msg = T("login.studioOff", "message", msg)
	}
	v.showMessage(msg, toneDanger)
}

func (v *loginView) setBusy(busy bool, step string) {
	if busy {
		v.connectBtn.SetText(T("common.cancel"))
		v.connectBtn.SetIcon(theme.CancelIcon())
		v.connectBtn.Importance = widget.MediumImportance
		v.progressRow.Show()
		v.activity.Start()
	} else {
		v.connectBtn.SetText(T("login.connect"))
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
		v.url.setHint(T("login.urlRequired"), true)
		v.app.win.Canvas().Focus(v.url.entry)
		return
	}
	if hint, ok := describeURL(rawURL); !ok {
		v.url.setHint(hint, true)
		v.app.win.Canvas().Focus(v.url.entry)
		return
	}
	if token == "" {
		v.token.setHint(T("login.tokenRequired"), true)
		v.app.win.Canvas().Focus(v.token.entry)
		return
	}
	remember := v.remember.Checked
	v.clearMessage()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	v.cancel = cancel
	v.setBusy(true, T("connection.contacting", "url", rawURL))

	a := v.app
	go func() {
		defer cancel()
		c := api.New(rawURL, token)
		step := func(s string) { onUI(func() { v.progress.SetText(s) }) }
		done := func(err error) {
			onUI(func() {
				v.cancel = nil
				v.setBusy(false, "")
				switch {
				case errors.Is(err, context.Canceled):
					v.showMessage(T("login.cancelled"), toneNeutral)
				case errors.Is(err, context.DeadlineExceeded):
					v.url.setHint(T("login.noAnswer"), true)
					v.showMessage(T("login.timedOut", "url", rawURL), toneDanger)
				default:
					v.fail(err)
				}
			})
		}
		if err := c.Ping(ctx); err != nil {
			done(err)
			return
		}
		step(T("connection.tokenAccepted"))
		cat, err := c.Catalog(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				err = loginCauseError{msg: T("connection.studioUnavailable", "message", err.Error()), cause: err}
			}
			done(err)
			return
		}
		onUI(func() {
			v.cancel = nil
			a.api = c
			a.catalog = cat
			saved := ""
			if remember {
				saved = token
			}
			a.cfg.RememberConnection(c.DisplayURL, saved, remember)
			_ = appconfig.Save(a.cfg)
			a.showMain()
			a.startHeartbeat()
		})
	}()
}
