// Package ui is the Fyne desktop interface for AgentFlow Studio.
package ui

import (
	"context"
	"image/color"
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
	"agentflow-studio/internal/docs"
	"agentflow-studio/internal/i18n"
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
	// status bar cells, refreshed by the heartbeat; see statusbar.go.
	statusState *canvas.Text
	statusFacts *canvas.Text
	themeBtn    *widget.Button
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
	// helpAction is F1 on screens with their own reference (the builder's
	// docs dock); elsewhere F1 opens the documentation.
	helpAction func()
	// unsaved reports whether the current screen holds edits that building it
	// again would lose (the builder, the config editor); nil means none.
	// reshow opens the current screen again as it is now, for screens that
	// take an argument (which agent, which page); nil means the rail's own
	// entry is enough. relocalizeView rebuilds a view that lives outside the
	// shell (login). A language change reads all three; see language.go.
	unsaved        func() bool
	reshow         func()
	relocalizeView func()

	shortcutsInstalled bool
}

func New(fyneApp fyne.App, win fyne.Window, cfg appconfig.Config) *App {
	variant := themeFromConfig(cfg.Theme, fyneApp.Settings().ThemeVariant())
	family := paletteFromConfig(cfg.Palette)
	i18n.SetLanguage(languageFromConfig(cfg.Language))
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
	if a.themeBtn != nil {
		a.themeBtn.SetIcon(a.themeToggleIcon())
	}
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
	clearTips()
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
		onUI(func() { a.catalog = catalog })
	}()
}

func (a *App) clearActions() {
	a.saveAction = nil
	a.sendAction = nil
	a.backAction = nil
	a.refreshAction = nil
	a.helpAction = nil
	a.unsaved = nil
	a.reshow = nil
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

// showProfileSwitcher lists the saved connections under the command bar's
// connection pill: switching cells is a click, and the token for the new cell
// comes from the saved profile rather than the clipboard.
func (a *App) showProfileSwitcher(anchor fyne.CanvasObject) {
	if a.win == nil || a.win.Canvas() == nil {
		return
	}
	items := vstack(2)
	var pop *widget.PopUp
	hide := func() {
		if pop != nil {
			pop.Hide()
		}
	}
	items.Add(container.New(layout.NewCustomPaddedLayout(2, 2, spaceS, spaceS), fieldLabel(strings.ToUpper(T("shell.connections")))))
	for _, p := range a.cfg.Profiles {
		p := p
		active := p.Name == a.cfg.Active
		name := widget.NewLabelWithStyle(p.Name, fyne.TextAlignLeading, fyne.TextStyle{Bold: active})
		name.Truncation = fyne.TextTruncateEllipsis
		tn := toneNeutral
		if active {
			tn = toneSuccess
		}
		var trail fyne.CanvasObject = hSpacer(spaceXS)
		if active {
			trail = container.NewCenter(chip(T("shell.activeConnection"), toneSuccess))
		}
		row := container.NewBorder(nil, nil,
			container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), vCenter(dotBox(dot(tn)))),
			container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), trail),
			vstack(0, compactLabel(name, false), compactLabel(oneLine(p.URL), false)))
		surf := newTapSurface(container.New(layout.NewCustomPaddedLayout(spaceXS, spaceXS, 0, 0), row), 7, false, nil, func() {
			hide()
			if !active {
				a.switchProfile(p.Name)
			}
		})
		items.Add(surf)
	}
	if len(a.cfg.Profiles) == 0 {
		items.Add(container.New(layout.NewCustomPaddedLayout(2, 2, spaceS, spaceS), caption(T("shell.noConnections"))))
	}
	items.Add(widget.NewSeparator())
	menuRow := func(label string, icon fyne.Resource, fn func()) fyne.CanvasObject {
		l := widget.NewLabel(label)
		row := container.NewBorder(nil, nil,
			container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), container.NewCenter(container.NewGridWrap(fyne.NewSize(15, 15), widget.NewIcon(icon)))),
			nil, compactLabel(l, false))
		s := newTapSurface(container.New(layout.NewCustomPaddedLayout(spaceXS+1, spaceXS+1, 0, spaceS), row), 7, false, nil, func() {
			hide()
			fn()
		})
		return s
	}
	items.Add(menuRow(T("shell.useAnotherConnection"), theme.ContentAddIcon(), func() {
		// Keep the saved profiles; clear only the active mirror so the login
		// screen starts blank and a successful login appends a profile.
		a.cfg.URL = ""
		a.cfg.Token = ""
		a.cfg.Active = ""
		a.showLogin("")
	}))
	items.Add(menuRow(T("shell.connectionSettings"), theme.SettingsIcon(), a.showSettings))
	items.Add(menuRow(T("shell.lockForgetToken"), theme.LogoutIcon(), a.lock))

	body := widthBox(300, items)
	pop = widget.NewPopUp(container.New(layout.NewCustomPaddedLayout(spaceXS, spaceXS, spaceXS, spaceXS), body), a.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	x := pos.X + anchor.Size().Width - 300 - 2*spaceXS
	if x < spaceS {
		x = spaceS
	}
	pop.ShowAtPosition(fyne.NewPos(x, pos.Y+anchor.Size().Height+spaceXS))
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

// The shell is three bands around the content: a command bar (where am I,
// jump anywhere, which runtime), a compact rail, and a status bar with the
// runtime's identity. Nothing in the chrome scrolls at the default size.
const (
	railWidth    = float32(216)
	topBarHeight = float32(44)
)

func (a *App) showMain() {
	a.online = true
	a.buildShell()
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

// buildShell builds the chrome around an empty content area and makes it the
// window's content. showMain calls it once after a login; a language change
// calls it again, because the chrome's text is built into its widgets.
func (a *App) buildShell() {
	a.railVisible = true
	a.relocalizeView = nil
	a.nav = a.buildNav()

	// ---- rail ---------------------------------------------------------------
	items := container.NewVBox()
	for _, section := range navSections {
		first := true
		for _, it := range a.nav {
			if it.section != section {
				continue
			}
			if first {
				if section != "Home" {
					items.Add(railSection(sectionLabel(section)))
				}
				first = false
			}
			it.row = newRailRow(it.label, it.icon, it.keyLabel, it.show)
			items.Add(it.row)
		}
	}
	// The destinations still scroll on a very short window; at the default
	// size they all fit.
	navScroll := container.NewVScroll(items)
	a.navScroll = navScroll

	// The rail lists screens and nothing else: creating an agent lives on the
	// Agents screen, in the command palette and on Ctrl+N.
	railSpacer := canvas.NewRectangle(color.Transparent)
	railSpacer.SetMinSize(fyne.NewSize(railWidth, 0))
	rail := navScroll
	railBG := canvas.NewRectangle(colorFloat)
	railEdge := canvas.NewRectangle(colorBorder)
	railEdge.SetMinSize(fyne.NewSize(1, 0))
	railBody := container.NewStack(railSpacer, rail)
	railPane := container.NewStack(railBG,
		container.NewBorder(nil, nil, nil, railEdge,
			container.New(layout.NewCustomPaddedLayout(spaceM, spaceS, spaceS, spaceS), railBody)))
	a.railPane = railPane

	// ---- command bar ----------------------------------------------------------
	a.breadcrumb = widget.NewLabel(T("nav.overview"))
	a.breadcrumb.SizeName = theme.SizeNameText

	searchHint := widget.NewLabel(T("shell.goToHint"))
	searchHint.Importance = widget.LowImportance
	searchHint.Truncation = fyne.TextTruncateEllipsis
	searchRow := container.NewBorder(nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0),
			container.NewCenter(container.NewGridWrap(fyne.NewSize(14, 14), widget.NewIcon(theme.SearchIcon())))),
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS-2), vCenter(kbd("Ctrl+K"))),
		vLeft(container.New(compactLabelLayout{}, searchHint)))
	search := newTapSurface(searchRow, 5, true, func() color.Color { return colorBG }, a.showCommandPalette)
	search.height = controlHeight

	a.railDot = dot(toneSuccess)
	a.connLabel = widget.NewLabel(a.connectionName())
	a.connLabel.SizeName = theme.SizeNameCaptionText
	a.connLabel.Truncation = fyne.TextTruncateEllipsis
	connRow := container.NewBorder(nil, nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, spaceS+2, 2), vCenter(dotBox(a.railDot))),
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceXS+2),
			container.NewCenter(container.NewGridWrap(fyne.NewSize(13, 13), widget.NewIcon(theme.MenuDropDownIcon())))),
		vLeft(container.New(compactLabelLayout{}, a.connLabel)))
	var connPill *tapSurface
	connPill = newTapSurface(connRow, 5, false, func() color.Color { return colorFloat }, func() {
		a.showProfileSwitcher(connPill)
	})
	connPill.height = controlHeight

	a.themeBtn = iconButton(a.themeToggleIcon(), T("shell.toggleTheme"), a.toggleTheme)
	cluster := hstack(spaceS,
		vCenter(widthBox(256, search)),
		vCenter(widthBox(190, connPill)),
		vCenter(a.languageToggle()),
		hstack(0,
			vCenter(iconButton(theme.HelpIcon(), T("shell.withShortcut", "label", T("nav.docs"), "key", "F1"), a.openDocs)),
			vCenter(a.themeBtn),
			vCenter(iconButton(theme.SettingsIcon(), T("nav.settings"), a.showSettings)),
		),
	)
	// 300 + 190 + three icon buttons and gaps: the part of the bar Fyne owns.
	clusterWidth := float32(256+190+3*32+4*spaceS+spaceS) + languageToggleWidth

	var topActions, topCenter fyne.CanvasObject = container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), cluster), nil
	if drag := a.dragHandle(); drag != nil {
		topCenter = drag
		controls := a.windowControls()
		topActions = container.NewHBox(cluster, hSpacer(spaceS), controls)
		clusterWidth += spaceS + controls.MinSize().Width
	}
	setChromeActionsWidth(clusterWidth)

	breadcrumbRow := hstack(spaceXS, a.railLogoButton(), vCenter(a.breadcrumb))
	topBarBG := canvas.NewRectangle(colorFloat)
	topHeight := canvas.NewRectangle(color.Transparent)
	topHeight.SetMinSize(fyne.NewSize(0, topBarHeight))
	topRow := container.NewStack(
		topBarBG,
		topHeight,
		container.New(layout.NewCustomPaddedLayout(0, 0, spaceS+2, 0),
			container.NewBorder(nil, nil,
				container.NewCenter(breadcrumbRow), topActions, topCenter)),
	)
	topEdge := canvas.NewRectangle(colorBorder)
	topEdge.SetMinSize(fyne.NewSize(0, 1))
	topBar := container.NewVBox(topRow, topEdge)

	a.content = container.NewStack()
	a.shell = container.NewStack(
		canvas.NewRectangle(colorBG),
		container.NewBorder(topBar, a.newStatusBar(), railPane, nil, a.content),
		// Last, so tooltips draw above everything; nothing in it takes input.
		newTipLayer(),
	)
	a.win.SetContent(a.shell)
	a.paintOnline(a.online)
}

// connectionName is what the command bar's pill shows: the saved profile's
// name when there is one, otherwise the address.
func (a *App) connectionName() string {
	if strings.TrimSpace(a.cfg.Active) != "" {
		return a.cfg.Active
	}
	if a.api != nil {
		return strings.TrimPrefix(strings.TrimPrefix(a.api.DisplayURL, "http://"), "https://")
	}
	return T("shell.notConnected")
}

func (a *App) themeToggleIcon() fyne.Resource {
	if a.variant == themeLight {
		return darkModeIcon
	}
	return lightModeIcon
}

// toggleTheme flips light/dark from the command bar.
func (a *App) toggleTheme() {
	if a.variant == themeLight {
		a.chooseTheme(themeDark)
	} else {
		a.chooseTheme(themeLight)
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

// startHeartbeat polls /admin/api/state so the connection dot tells the
// truth and the status bar shows which runtime this is: a runtime that
// restarts or goes away turns the dot red with a toast, and coming back turns
// it green again. It runs from a successful connect until lock.
func (a *App) startHeartbeat() {
	a.stopHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	a.heartbeatCancel = cancel
	c := a.api
	beat := func() bool {
		pctx, pcancel := context.WithTimeout(ctx, 8*time.Second)
		st, err := c.State(pctx)
		pcancel()
		if ctx.Err() != nil {
			return false
		}
		onUI(func() {
			a.setOnline(err)
			if err == nil {
				a.setRuntimeFacts(st)
			}
		})
		return true
	}
	go func() {
		if !beat() {
			return
		}
		tick := time.NewTicker(15 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
			}
			if !beat() {
				return
			}
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
	a.paintOnline(ok)
	if ok == a.online {
		return
	}
	a.online = ok
	if ok {
		a.setStatus(T("shell.runtimeReachable"), toneSuccess)
	} else {
		a.setStatus(T("shell.runtimeUnreachable", "message", err.Error()), toneDanger)
	}
}

// paintOnline shows the connection state in the command bar and status bar.
func (a *App) paintOnline(ok bool) {
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
			a.connLabel.SetText(a.connectionName())
		} else {
			a.connLabel.SetText(T("shell.offlineName", "name", a.connectionName()))
		}
	}
	a.setStatusBarState(ok)
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
		onUI(func() {
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
	a.setStatus(T("common.error", "message", err.Error()), toneDanger)
}
