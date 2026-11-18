package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
)

// screen is the standard page every operator screen is built from: a header
// with actions, a meta line, a banner for errors and notices, and a body that
// is either a scrolling stack or a caller-supplied layout. It owns the three
// states every screen has besides its content: loading, failed and empty.
type screen struct {
	app    *App
	root   fyne.CanvasObject
	banner *banner
	meta   *widget.Label
	body   *fyne.Container
}

// newScreen builds a page whose body scrolls vertically.
func (a *App) newScreen(title, subtitle string, actions fyne.CanvasObject) *screen {
	s := &screen{app: a, banner: newBanner(), meta: oneLine(""), body: vstack(spaceS)}
	s.root = s.frame(title, subtitle, actions, container.NewVScroll(s.body))
	return s
}

// newPane builds a page whose body is the given layout (split views, tabs).
func (a *App) newPane(title, subtitle string, actions, content fyne.CanvasObject) *screen {
	s := &screen{app: a, banner: newBanner(), meta: oneLine(""), body: vstack(spaceS)}
	s.root = s.frame(title, subtitle, actions, content)
	return s
}

func (s *screen) frame(title, subtitle string, actions, content fyne.CanvasObject) fyne.CanvasObject {
	head := pageHeader(title, subtitle, actions)
	return container.NewBorder(
		vstack(0, pageInset(vstack(spaceS, head, s.meta)), hInset(s.banner), widget.NewSeparator()),
		nil, nil, nil,
		pageInset(content),
	)
}

func (s *screen) refresh() {
	if s.root != nil {
		s.root.Refresh()
	}
}

func (s *screen) notice(text string, t tone) {
	s.banner.Set(text, t)
	s.refresh()
}

func (s *screen) setBody(objs ...fyne.CanvasObject) {
	clearBox(s.body)
	for _, o := range objs {
		s.body.Add(o)
	}
	s.body.Refresh()
	s.refresh()
}

func (s *screen) loading(text string) {
	if text == "" {
		text = "Loading…"
	}
	act := widget.NewActivity()
	act.Start()
	s.setBody(vSpacer(spaceXL), container.NewCenter(hstack(spaceS, act, muted(text))))
}

// fail shows err in the banner and a retry button in the body. A runtime
// feature that is switched off (503) reads as guidance rather than an error.
func (s *screen) fail(err error, retry func()) {
	if msg, off := featureOff(err); off {
		s.banner.Clear()
		s.setBody(layout.NewSpacer(), emptyStateCard("Not enabled on this runtime", msg, "", nil), layout.NewSpacer())
		return
	}
	s.notice(err.Error(), toneDanger)
	var objs []fyne.CanvasObject
	if retry != nil {
		objs = append(objs, vSpacer(spaceL), container.NewCenter(actionButton("Retry", theme.ViewRefreshIcon(), retry)))
	}
	s.setBody(objs...)
}

func (s *screen) empty(title, guidance, label string, action func()) {
	s.setBody(layout.NewSpacer(), emptyStateCard(title, guidance, label, action), layout.NewSpacer())
}

// featureOff reports whether err is the runtime saying a subsystem is not
// enabled (503), and turns the message into guidance.
func featureOff(err error) (string, bool) {
	var ae *api.Error
	if !errors.As(err, &ae) || ae.Status != 503 {
		return "", false
	}
	msg := ae.Message
	switch {
	case strings.Contains(msg, "identity"):
		msg += "\n\nSet runtime.identity.enabled: true (and runtime.users.enabled for the public user API), then restart AgentFlow."
	case strings.Contains(msg, "tenancy"):
		msg += "\n\nTenancy needs a runtime store (runtime.persistence)."
	}
	return msg, true
}

// onUI runs fn on the UI goroutine and does not wait for it: by the time onUI
// returns, fn has usually not run yet. Every background completion in this
// package goes through it (or onUIWait) rather than calling Fyne directly, for
// one reason: Fyne's headless test driver has no UI goroutine and runs such a
// callback on whichever goroutine called it. A test that builds views on its
// own goroutine would then have a finished background load touching widgets
// while the test is still laying them out. The package's tests install a
// runner (harness_test.go) that makes the running test the UI goroutine
// instead.
func onUI(fn func()) {
	if run := uiRunner.Load(); run != nil {
		(*run)(fn, false)
		return
	}
	fyne.Do(fn)
}

// onUIWait runs fn on the UI goroutine and returns once it has run. It is for
// a background goroutine that needs something back — the editor's text, say —
// before it can continue. It must not be called on the UI goroutine itself.
func onUIWait(fn func()) {
	if run := uiRunner.Load(); run != nil {
		(*run)(fn, true)
		return
	}
	fyne.DoAndWait(fn)
}

// uiRunner, when set, receives every onUI and onUIWait callback instead of
// Fyne; wait says whether the caller needs fn to have run before the runner
// returns. Only tests set it. It is atomic because background goroutines read
// it while a test installs it.
var uiRunner atomic.Pointer[func(fn func(), wait bool)]

// async runs call off the UI goroutine and delivers its result on it.
func async[T any](timeout time.Duration, call func(context.Context) (T, error), done func(T, error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		v, err := call(ctx)
		onUI(func() { done(v, err) })
	}()
}

// asyncErr is async for calls that return only an error.
func asyncErr(timeout time.Duration, call func(context.Context) error, done func(error)) {
	async(timeout, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, call(ctx)
	}, func(_ struct{}, err error) { done(err) })
}

// ---- data presentation --------------------------------------------------------

// tableColumn is one column of a simpleTable: a header and a fixed width
// (0 = take the remaining space; at most one column should).
type tableColumn struct {
	title string
	width float32
	right bool
}

// simpleTable lays rows out on fixed column widths with a header row and
// hairline separators. Cells are canvas objects so rows can carry chips and
// buttons, not only text.
func simpleTable(cols []tableColumn, rows [][]fyne.CanvasObject) fyne.CanvasObject {
	line := func(cells []fyne.CanvasObject) fyne.CanvasObject {
		var lead []fyne.CanvasObject
		var fill fyne.CanvasObject
		var trail []fyne.CanvasObject
		seenFill := false
		for i, c := range cols {
			var cell fyne.CanvasObject = layout.NewSpacer()
			if i < len(cells) && cells[i] != nil {
				cell = cells[i]
			}
			if c.width == 0 && !seenFill {
				fill = cell
				seenFill = true
				continue
			}
			boxed := widthBox(c.width, cell)
			if seenFill {
				trail = append(trail, boxed)
			} else {
				lead = append(lead, boxed)
			}
		}
		return container.NewBorder(nil, nil, hstack(spaceS, lead...), hstack(spaceS, trail...), fill)
	}
	header := make([]fyne.CanvasObject, len(cols))
	for i, c := range cols {
		l := fieldLabel(sentenceCase(c.title))
		if c.right {
			l.Alignment = fyne.TextAlignTrailing
		}
		header[i] = l
	}
	out := vstack(0, line(header), widget.NewSeparator())
	for _, r := range rows {
		out.Add(container.New(layout.NewCustomPaddedLayout(2, 2, 0, 0), line(r)))
		out.Add(widget.NewSeparator())
	}
	return out
}

func cellText(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapOff
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

func cellMono(text string) *widget.Label {
	l := mono(text, theme.SizeNameText)
	return l
}

func cellNum(n int64) *widget.Label {
	l := cellMono(humanCount(n))
	l.Alignment = fyne.TextAlignTrailing
	return l
}

// statTile is a compact KPI: label, big value, optional sub-line and spark.
func statTile(label, value, sub string, t tone, spark fyne.CanvasObject) fyne.CanvasObject {
	v := titleText(value, theme.SizeNameHeadingText)
	// A number is coloured only when it asks for attention.
	if t != toneNeutral && t != toneAccent {
		v.Color = toneColor(t)
	}
	items := []fyne.CanvasObject{fieldLabel(label), insetLeading(v)}
	if sub != "" {
		items = append(items, compactLabel(caption(sub), false))
	}
	if spark != nil {
		items = append(items, spark)
	}
	return panel(vstack(2, items...))
}

// sparkline draws a counter series as a polyline scaled to its own range.
type sparkline struct {
	widget.BaseWidget
	values []int64
	height float32
}

func newSparkline(values []int64, height float32) *sparkline {
	s := &sparkline{values: values, height: height}
	s.ExtendBaseWidget(s)
	return s
}

func (s *sparkline) SetValues(v []int64) {
	s.values = v
	s.Refresh()
}

func (s *sparkline) MinSize() fyne.Size { return fyne.NewSize(60, s.height) }

func (s *sparkline) CreateRenderer() fyne.WidgetRenderer {
	r := &sparkRenderer{s: s, base: canvas.NewRectangle(color.Transparent)}
	return r
}

type sparkRenderer struct {
	s     *sparkline
	base  *canvas.Rectangle
	lines []fyne.CanvasObject
	size  fyne.Size
}

func (r *sparkRenderer) Layout(size fyne.Size) {
	r.size = size
	r.base.Resize(size)
	r.rebuild()
}

func (r *sparkRenderer) rebuild() {
	vals := r.s.values
	r.lines = r.lines[:0]
	if len(vals) < 2 || r.size.Width <= 0 {
		return
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo = min(lo, v)
		hi = max(hi, v)
	}
	// Scale to the series' own range, but never magnify a change smaller
	// than a tenth of the value: a counter ticking 4→6 should read as nearly
	// flat, not as a sawtooth.
	span := math.Max(float64(hi-lo), math.Max(float64(hi)/10, 1))
	w, h := float64(r.size.Width), float64(r.size.Height)-2
	step := w / float64(len(vals)-1)
	pt := func(i int) fyne.Position {
		y := h - (float64(vals[i]-lo)/span)*h + 1
		return fyne.NewPos(float32(float64(i)*step), float32(math.Max(1, y)))
	}
	for i := 1; i < len(vals); i++ {
		l := canvas.NewLine(colorAccent)
		l.StrokeWidth = 1.5
		l.Position1, l.Position2 = pt(i-1), pt(i)
		r.lines = append(r.lines, l)
	}
}

func (r *sparkRenderer) MinSize() fyne.Size { return r.s.MinSize() }
func (r *sparkRenderer) Refresh() {
	r.rebuild()
	for _, o := range r.lines {
		o.Refresh()
	}
	canvas.Refresh(r.s)
}
func (r *sparkRenderer) Objects() []fyne.CanvasObject {
	return append([]fyne.CanvasObject{r.base}, r.lines...)
}
func (r *sparkRenderer) Destroy() {}

// meterBar shows used against limit; limit <= 0 renders an "unlimited" chip.
func meterBar(used, limit int64) fyne.CanvasObject {
	if limit <= 0 {
		return hstack(spaceXS, vCenter(cellMono(humanCount(used))), vCenter(chip("unlimited", toneNeutral)))
	}
	frac := float64(used) / float64(limit)
	bar := widget.NewProgressBar()
	bar.Max = 1
	bar.SetValue(math.Min(frac, 1))
	bar.TextFormatter = func() string {
		return fmt.Sprintf("%s / %s", humanCount(used), humanCount(limit))
	}
	return bar
}

// ---- formatting -----------------------------------------------------------------

func humanCount(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	var s string
	switch {
	case n >= 1_000_000_000:
		s = fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		s = fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		s = fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		s = fmt.Sprintf("%d", n)
	}
	s = strings.Replace(s, ".0", "", 1)
	if neg {
		return "-" + s
	}
	return s
}

func humanDuration(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", seconds)
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// relTime renders a unix timestamp as "3m ago" (or a date beyond a week).
func relTime(unix int64) string {
	if unix <= 0 {
		return "—"
	}
	t := time.Unix(unix, 0)
	d := time.Since(t)
	switch {
	case d < 0:
		return t.Local().Format("2006-01-02 15:04")
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours())/24)
	}
	return t.Local().Format("2006-01-02")
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

// copyButton puts text on the clipboard and confirms with a toast.
func (a *App) copyButton(text, what string) *widget.Button {
	return iconButton(theme.ContentCopyIcon(), "Copy", func() {
		if a.fyneApp != nil && a.fyneApp.Clipboard() != nil {
			a.fyneApp.Clipboard().SetContent(text)
		} else if a.win != nil {
			a.win.Clipboard().SetContent(text)
		}
		a.setStatus(what+" copied", toneSuccess)
	})
}

// promptText asks for one value in a modal and calls onOK with it.
func (a *App) promptText(title, subtitle, label, placeholder, initial, okLabel string, validate func(string) error, onOK func(string)) {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(placeholder)
	entry.SetText(initial)
	errBanner := newBanner()
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}
	submit := func() {
		v := strings.TrimSpace(entry.Text)
		if validate != nil {
			if err := validate(v); err != nil {
				errBanner.Set(err.Error(), toneDanger)
				return
			}
		}
		closePopup()
		onOK(v)
	}
	entry.OnSubmitted = func(string) { submit() }
	form := vstack(spaceS, vstack(spaceXS, fieldLabel(label), controlFrame(entry)), errBanner)
	cancel := widthBox(100, controlFrame(actionButton("Cancel", nil, closePopup)))
	ok := widthBox(140, controlFrame(primaryButton(okLabel, nil, submit)))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancel, ok), nil)
	popup = showModalCard(a.win, title, subtitle, widthBox(520, form), footer)
	if c := a.win.Canvas(); c != nil && popup != nil {
		c.Focus(entry)
	}
}

// loadUserIDs fetches profile ids for pickers. A runtime without the identity
// layer simply yields none, and the picker stays a free-text field.
func (a *App) loadUserIDs(done func([]string)) {
	if a.api == nil {
		return
	}
	async(10*time.Second, a.api.Users, func(l api.UserList, err error) {
		if err != nil || len(l.Users) == 0 {
			return
		}
		ids := make([]string, 0, len(l.Users))
		for _, u := range l.Users {
			ids = append(ids, u.UserID)
		}
		done(ids)
	})
}

func requireNonEmpty(what string) func(string) error {
	return func(v string) error {
		if v == "" {
			return fmt.Errorf("%s is required", what)
		}
		return nil
	}
}
