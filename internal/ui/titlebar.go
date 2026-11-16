package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// dragBar is the empty center of the custom title bar. Dragging it moves the
// native window by tracking the cursor in screen coordinates; double-clicking
// toggles maximize.
type dragBar struct {
	widget.BaseWidget
	win      fyne.Window
	dragging bool
	offX     int32
	offY     int32
}

func newDragBar(win fyne.Window) *dragBar {
	d := &dragBar{win: win}
	d.ExtendBaseWidget(d)
	return d
}

func (d *dragBar) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (d *dragBar) MinSize() fyne.Size { return fyne.NewSize(10, 44) }

func (d *dragBar) Dragged(*fyne.DragEvent) {
	title := d.win.Title()
	if !d.dragging {
		offX, offY, ok := windowDragOffsets(title)
		if !ok {
			return
		}
		d.offX, d.offY, d.dragging = offX, offY, true
	}
	x, y, ok := windowCursor()
	if !ok {
		return
	}
	windowMove(title, x-d.offX, y-d.offY)
}

func (d *dragBar) DragEnd() { d.dragging = false }

func (d *dragBar) DoubleTapped(*fyne.PointEvent) {
	windowToggleMaximize(d.win.Title())
}

// windowControls are the frameless window's sidebar toggle (main screen only),
// minimize / maximize / close. They are folded into the existing top bar and a
// floating cluster on the welcome screen, so there is no separate title bar.
// railLogoButton is the logo in the window bar. It becomes the sidebar toggle
// on hover, and is a normal logo otherwise.
func (a *App) railLogoButton() fyne.CanvasObject {
	logo := logoMark(22)
	btn := newChromeButton(glyphSidebar, a.toggleRail)
	// This control lives in the client area; never feed the native window hit
	// regions used by the minimize/maximize/close buttons.
	btn.setID = ^uint64(0)
	btn.SetGlyphVisible(false)
	btn.SetMirrored(true) // rail starts open: the chevron points left
	a.railToggle = btn
	btn.onHover = func(hovered bool) {
		btn.SetGlyphVisible(hovered)
		if hovered {
			logo.Hide()
		} else {
			logo.Show()
		}
		logo.Refresh()
	}
	// A compact 26px hover target keeps the logo visually close to the
	// breadcrumb while still giving the control a comfortable click area.
	buttonCell := container.NewGridWrap(fyne.NewSize(26, 26), btn)
	return container.NewStack(container.NewCenter(logo), buttonCell)
}

func (a *App) windowControls() fyne.CanvasObject {
	if !borderlessSupported() {
		return nil
	}

	setID := newChromeControlSet()
	minimize := newChromeButton(glyphMinimize, func() { windowMinimize(a.win.Title()) })
	minimize.setID = setID
	var maximize *chromeButton
	maximize = newChromeButton(glyphMaximize, func() {
		windowToggleMaximize(a.win.Title())
		if windowIsMaximized(a.win.Title()) {
			maximize.SetGlyph(glyphRestore)
		} else {
			maximize.SetGlyph(glyphMaximize)
		}
	})
	maximize.setID = setID
	closeBtn := newChromeButton(glyphClose, func() { a.win.Close() })
	closeBtn.setID = setID
	closeBtn.danger = true

	buttons := []*chromeButton{minimize, maximize, closeBtn}
	items := []fyne.CanvasObject{
		container.NewCenter(minimize),
		container.NewCenter(maximize),
		container.NewCenter(closeBtn),
	}
	registerChromeButtons(buttons...)

	// Windows owns the minimize/maximize/close hit regions, so hover feedback
	// arrives from WM_NCMOUSEMOVE. The trailing spacer insets the red close
	// surface from the window edge.
	onChromeHoverChanged(func(c chromeControl) {
		minimize.SetHovered(c == controlMinimize)
		maximize.SetHovered(c == controlMaximize)
		closeBtn.SetHovered(c == controlClose)
	})
	setSidebarHitRegion(false)

	// The native click never reaches Fyne; WM_SIZE keeps the glyph in sync.
	onWindowZoomChanged(func(max bool) {
		if max {
			maximize.SetGlyph(glyphRestore)
		} else {
			maximize.SetGlyph(glyphMaximize)
		}
	})
	if windowIsMaximized(a.win.Title()) {
		maximize.SetGlyph(glyphRestore)
	}

	margin := canvas.NewRectangle(color.Transparent)
	margin.SetMinSize(fyne.NewSize(chromeControlsMargin, 0))
	return container.NewHBox(append(items, margin)...)
}

// dragHandle is the transparent region that moves the window. It returns nil
// on platforms with a native frame.
func (a *App) dragHandle() fyne.CanvasObject {
	if !borderlessSupported() {
		return nil
	}
	return newDragBar(a.win)
}

// chromeStrip is the welcome screen's invisible drag/control row. It has no
// background or border, so the window stays visually frameless.
func (a *App) chromeStrip() fyne.CanvasObject {
	if !borderlessSupported() {
		return nil
	}
	// The welcome screen has no command bar: only the window buttons are
	// Fyne's, so the hit test goes back to its default split.
	setChromeActionsWidth(0)
	return container.NewBorder(nil, nil,
		nil,
		container.NewPadded(a.windowControls()),
		a.dragHandle())
}
