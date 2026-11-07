// Package ui is the Fyne desktop interface for AgentFlow Studio.
package ui

import (
	"context"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"agentflow-studio/internal/api"
	"agentflow-studio/internal/appconfig"
	"agentflow-studio/internal/docs"
)

const appVersion = "0.1.0"

// App owns the window, the connection and the current view.
type App struct {
	fyneApp fyne.App
	win     fyne.Window
	cfg     appconfig.Config
	api     *api.Client
	catalog api.Catalog

	content     *fyne.Container
	shell       *fyne.Container
	breadcrumb  *widget.Label
	railDot     *canvas.Circle
	railPane    fyne.CanvasObject
	railToggle  *chromeButton
	railVisible bool
	variant     appTheme
	palette     themePalette
	luaEditor   *luaEditor
	docs        *docs.Library

	// nav is the shell's destination list; see nav.go.
	nav       []*navItem
	connLabel *widget.Label
	navScroll *container.Scroll
	// online is the heartbeat's last verdict; heartbeatCancel stops it.
	online          bool
	heartbeatCancel context.CancelFunc

	view       string
	toast      *widget.PopUp
	toastTimer *time.Timer

	chatCancel context.CancelFunc
	// viewCancel stops the polling or streaming the current screen started.
	viewCancel    context.CancelFunc
	saveAction    func()
	sendAction    func()
	backAction    func()
	refreshAction func()

	shortcutsInstalled bool
}

func New(fyneApp fyne.App, win fyne.Window, cfg appconfig.Config) *App {
	variant := themeFromConfig(cfg.Theme, fyneApp.Settings().ThemeVariant())
	family := paletteFromConfig(cfg.Palette)
	applyStudioTheme(fyneApp, family, variant)
	return &App{fyneApp: fyneApp, win: win, cfg: cfg, variant: variant, palette: family}
}

func (a *App) chooseTheme(next appTheme) {
	if a.variant == next {
		return
	}
	a.applyTheme(next)
	a.cfg.Theme = next.String()
	_ = appconfig.Save(a.cfg)
}

func (a *App) choosePalette(next themePalette) {
	if a.palette == next {
		return
	}
	a.applyPalette(next)
	a.cfg.Palette = next.String()
	_ = appconfig.Save(a.cfg)
}

// applyTheme switches light/dark mode in place while keeping the active palette.
func (a *App) applyTheme(v appTheme) {
	a.applyAppearance(a.palette, v)
}

// applyPalette switches the color theme family in place while keeping light/dark mode.
func (a *App) applyPalette(p themePalette) {
	a.applyAppearance(p, a.variant)
}

// applyAppearance switches palettes and luminance modes in place. Canvas colors
// are remapped and widgets refresh from the new theme, so the current screen
// and its unsaved state survive the switch.
func (a *App) applyAppearance(p themePalette, v appTheme) {
	old := activePalette
	setTheme(p, v)
	a.palette = p
	a.variant = v
	a.fyneApp.Settings().SetTheme(studioTheme{})
	a.fyneApp.SetIcon(appIconResource())
	if a.win != nil && a.win.Canvas() != nil && a.win.Canvas().Content() != nil {
		remapThemeColors(a.win.Canvas().Content(), old, activePalette)
		a.win.Canvas().Refresh(a.win.Canvas().Content())
	}
}

// Start shows the login screen, reconnecting at once when a URL and token
// were remembered from the last session.
func (a *App) Start() {
	a.showLoginWith(loginOptions{auto: a.cfg.Token != "" && a.cfg.URL != ""})
}

func (a *App) setContent(obj fyne.CanvasObject) {
	a.win.SetContent(obj)
}

func (a *App) setMain(obj fyne.CanvasObject) {
	if a.content == nil {
		return
	}
	if c := a.win.Canvas(); c != nil {
		// Focus can outlive the widget that had it when the view is replaced;
		// a removed entry then swallows canvas shortcuts (Ctrl+1/2/3).
		c.Unfocus()
	}
	a.content.Objects = []fyne.CanvasObject{obj}
	a.content.Refresh()
}

// closeLuaEditor stops the language server owned by the current builder.
func (a *App) closeLuaEditor() {
	if a.luaEditor != nil {
		a.luaEditor.Close()
		a.luaEditor = nil
	}
}

func (a *App) cancelChat() {
	if a.chatCancel != nil {
		a.chatCancel()
		a.chatCancel = nil
	}
}

// reloadCatalog updates the builder's model/tool/option catalog after a model
// or agent edit. The page itself is not rebuilt; the next builder open reads
// the fresh catalog.
func (a *App) reloadCatalog() {
	if a.api == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		catalog, err := a.api.Catalog(ctx)
		if err != nil {
			return
		}
		fyne.Do(func() { a.catalog = catalog })
	}()
}

func (a *App) clearActions() {
	a.saveAction = nil
	a.sendAction = nil
	a.backAction = nil
	a.refreshAction = nil
}

func (a *App) lock() {
	a.closeLuaEditor()
	a.cancelChat()
	a.cancelView()
	a.stopHeartbeat()
	a.cfg.Token = ""
	_ = appconfig.Save(a.cfg)
	a.api = nil
	a.showLogin("")
}

// showProfileSwitcher lists the saved connections under the active one. It is
// the rail's account footer: switching cells is a click, and the token for the
// new cell comes from the saved profile rather than the clipboard.
func (a *App) showProfileSwitcher(anchor fyne.CanvasObject) {
	if a.win == nil || a.win.Canvas() == nil {
		return
	}
	items := container.NewVBox()
	var pop *widget.PopUp
	pop = widget.NewPopUp(items, a.win.Canvas())
	for _, p := range a.cfg.Profiles {
		p := p
		label := p.Name
		if p.Name == a.cfg.Active {
			label += "  ✓"
		}
		items.Add(widget.NewButton(label+"\n"+p.URL, func() {
			pop.Hide()
			a.switchProfile(p.Name)
		}))
	}
	if len(a.cfg.Profiles) == 0 {
		items.Add(caption("No saved connections yet."))
	}
	items.Add(widget.NewSeparator())
	items.Add(widget.NewButton("Use another connection…", func() {
		pop.Hide()
		// Keep the saved profiles; clear only the active mirror so the login
		// screen starts blank and a successful login appends a profile.
		a.cfg.URL = ""
		a.cfg.Token = ""
		a.cfg.Active = ""
		a.showLogin("")
	}))
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	pop.ShowAtPosition(fyne.NewPos(pos.X, pos.Y-a.win.Canvas().Size().Height/2))
}

// switchProfile makes a saved profile active and reconnects. A profile without
// a remembered token opens the login screen prefilled with its URL.
func (a *App) switchProfile(name string) {
	if !a.cfg.SelectProfile(name) {
		return
	}
	_ = appconfig.Save(a.cfg)
	a.closeLuaEditor()
	a.cancelChat()
	a.cancelView()
	a.stopHeartbeat()
	a.api = nil
	a.showLoginWith(loginOptions{auto: a.cfg.Token != "" && a.cfg.URL != ""})
}

// ---- main shell ------------------------------------------------------------

func (a *App) showMain() {
	a.railVisible = true
	a.online = true
	a.nav = a.buildNav()

	newAgent := newRailRow("New agent", theme.ContentAddIcon(), "Ctrl+N", a.showNewAgent)
	search := newRailRow("Go to…", theme.SearchIcon(), "Ctrl+K", a.showCommandPalette)
	items := container.NewVBox()
	for _, section := range navSections {
		first := true
		for _, it := range a.nav {
			if it.section != section {
				continue
			}
			if first {
				if section != "Home" {
					items.Add(railSection(section))
				}
				first = false
			}
			it.row = newRailRow(it.label, it.icon, it.keyLabel, it.show)
			items.Add(it.row)
		}
	}
	// The destinations scroll so a short window never clips them; the
	// actions above and the connection card below stay put.
	navScroll := container.NewVScroll(items)
	a.navScroll = navScroll

	a.railDot = dot(toneSuccess)
	a.connLabel = oneLine(a.api.DisplayURL)
	connectionRow := container.NewBorder(nil, nil,
		container.NewHBox(vCenter(dotBox(a.railDot))),
		hstack(spaceXS,
			iconButton(theme.SettingsIcon(), "Settings", a.showSettings),
			iconButton(theme.LogoutIcon(), "Lock", a.lock)),
		a.connLabel)
	var profileBtn *widget.Button
	profileBtn = actionButton(orDash(a.cfg.Active), theme.AccountIcon(), func() {
		a.showProfileSwitcher(profileBtn)
	})
	connection := container.NewVBox(profileBtn, subpanel(connectionRow))

	railWidth := canvas.NewRectangle(color.Transparent)
	railWidth.SetMinSize(fyne.NewSize(248, 0))

	rail := container.NewBorder(
		container.NewVBox(vSpacer(spaceXS), newAgent, search, widget.NewSeparator()),
		container.NewVBox(vSpacer(spaceXS), connection),
		nil, nil,
		navScroll,
	)
	railBG := canvas.NewRectangle(colorFloat)
	railBG.StrokeColor = colorBorder
	railBG.StrokeWidth = 1
	railBody := container.NewStack(railWidth, rail)
	railPane := container.NewStack(railBG, container.New(layout.NewCustomPaddedLayout(spaceS, spaceS, spaceS, spaceS), railBody))
	a.railPane = railPane

	a.breadcrumb = widget.NewLabel("Home / Overview")
	a.breadcrumb.TextStyle = fyne.TextStyle{Monospace: true}
	a.breadcrumb.SizeName = theme.SizeNameText

	var topActions, topCenter fyne.CanvasObject
	if drag := a.dragHandle(); drag != nil {
		topCenter = drag
		topActions = container.NewHBox(a.windowControls())
	}
	breadcrumbRow := hstack(spaceXS, a.railLogoButton(), vCenter(a.breadcrumb))
	topBarBG := canvas.NewRectangle(colorFloat)
	topRow := container.NewStack(
		topBarBG,
		container.New(layout.NewCustomPaddedLayout(2, 2, spaceS, 0),
			container.NewBorder(nil, nil,
				container.NewCenter(breadcrumbRow), topActions, topCenter)),
	)
	topBar := container.NewVBox(topRow, newTransStripe(0, 2))

	a.content = container.NewStack()
	a.shell = container.NewStack(
		canvas.NewRectangle(colorBG),
		container.NewBorder(topBar, nil, railPane, nil, a.content),
	)
	a.win.SetContent(a.shell)
	if !a.shortcutsInstalled {
		a.installShortcuts()
		a.shortcutsInstalled = true
	}
	a.win.Resize(fyne.NewSize(1280, 860))
	a.showOverview()
	if c := a.win.Canvas(); c != nil {
		c.Refresh(a.shell)
	}
}

// toggleRail collapses or restores the left navigation rail.
func (a *App) toggleRail() {
	if a.railPane == nil {
		return
	}
	a.railVisible = !a.railVisible
	if a.railVisible {
		a.railPane.Show()
	} else {
		a.railPane.Hide()
	}
	if a.railToggle != nil {
		a.railToggle.SetMirrored(a.railVisible)
	}
	if a.shell != nil {
		a.shell.Refresh()
	}
}

// startHeartbeat polls /admin/api/state so the rail's connection dot tells
// the truth: a runtime that restarts or goes away turns it red with a toast,
// and coming back turns it green again. It runs from a successful connect
// until lock.
func (a *App) startHeartbeat() {
	a.stopHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	a.heartbeatCancel = cancel
	c := a.api
	go func() {
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			pctx, pcancel := context.WithTimeout(ctx, 8*time.Second)
			err := c.Ping(pctx)
			pcancel()
			if ctx.Err() != nil {
				return
			}
			fyne.Do(func() { a.setOnline(err) })
		}
	}()
}

func (a *App) stopHeartbeat() {
	if a.heartbeatCancel != nil {
		a.heartbeatCancel()
		a.heartbeatCancel = nil
	}
}

// setOnline records a heartbeat result and announces transitions only.
func (a *App) setOnline(err error) {
	ok := err == nil
	if a.railDot != nil {
		if ok {
			a.railDot.FillColor = toneColor(toneSuccess)
		} else {
			a.railDot.FillColor = toneColor(toneDanger)
		}
		a.railDot.Refresh()
	}
	if a.connLabel != nil && a.api != nil {
		if ok {
			a.connLabel.SetText(a.api.DisplayURL)
		} else {
			a.connLabel.SetText("offline · " + a.api.DisplayURL)
		}
	}
	if ok == a.online {
		return
	}
	a.online = ok
	if ok {
		a.setStatus("runtime reachable again", toneSuccess)
	} else {
		a.setStatus("runtime unreachable: "+err.Error(), toneDanger)
	}
}

func (a *App) setBreadcrumb(text string) {
	if a.breadcrumb != nil {
		a.breadcrumb.SetText(text)
	}
}

func (a *App) setStatus(text string, t tone) {
	if text == "" || a.win == nil || a.win.Canvas() == nil {
		return
	}
	c := a.win.Canvas()
	label := widget.NewLabel(text)
	label.SizeName = theme.SizeNameCaptionText
	label.Wrapping = fyne.TextWrapWord
	bg := canvas.NewRectangle(toneBackground(t))
	bg.CornerRadius = 8
	bg.StrokeColor = toneColor(t)
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.NewPadded(label))
	if a.toast != nil {
		a.toast.Hide()
	}
	a.toast = widget.NewPopUp(card, c)
	min := card.MinSize()
	a.toast.ShowAtPosition(fyne.NewPos(c.Size().Width-min.Width-spaceL, c.Size().Height-min.Height-spaceL))
	if a.toastTimer != nil {
		a.toastTimer.Stop()
	}
	a.toastTimer = time.AfterFunc(2200*time.Millisecond, func() {
		fyne.Do(func() {
			if a.toast != nil {
				a.toast.Hide()
			}
		})
	})
}

func (a *App) showError(err error) {
	if err == nil {
		return
	}
	a.setStatus("Error: "+err.Error(), toneDanger)
}
