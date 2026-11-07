package ui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Window-control geometry. The buttons sit in an HBox, whose gaps follow
// theme.SizeNamePadding; chromeControlsMargin is the trailing spacer that keeps
// the close surface off the window edge. The native hit test uses the controls'
// laid-out rectangles, so only the size is needed here.
const (
	chromeButtonWidth    float32 = 34
	chromeButtonHeight   float32 = 34
	chromeControlsMargin float32 = 8
)

// chromeRect is a control's rectangle in Fyne canvas coordinates.
type chromeRect struct {
	X, Y, W, H float32
}

// chromeGeometry records where Fyne actually laid the controls out, plus the
// canvas size and scale. The native hit test uses this instead of deriving
// positions from the client edge: Windows' DPI can differ from Fyne's scale,
// and the GL viewport can be offset by the hidden frame.
type chromeGeometry struct {
	generation      uint64
	scale           float32
	canvasW         float32
	canvasH         float32
	min, max, close chromeRect
	sidebar         chromeRect
	hasSidebar      bool
}

var (
	chromeGeomMu sync.Mutex
	chromeGeom   chromeGeometry

	chromeButtonsMu sync.Mutex
	chromeButtons   []*chromeButton
)

// registerChromeButtons replaces the set whose positions feed the hit test.
func registerChromeButtons(buttons ...*chromeButton) {
	chromeButtonsMu.Lock()
	chromeButtons = append([]*chromeButton(nil), buttons...)
	chromeButtonsMu.Unlock()
}

// refreshChromeGeometry re-reads the controls' absolute positions. A window
// resize can shift their parent containers without moving the buttons
// themselves, so callers invoke this once the new layout has settled.
func refreshChromeGeometry() {
	chromeButtonsMu.Lock()
	buttons := append([]*chromeButton(nil), chromeButtons...)
	chromeButtonsMu.Unlock()
	for _, b := range buttons {
		b.updateGeometry()
	}
}

// newChromeControlSet starts a fresh geometry generation. Controls from a
// screen that is being replaced can lay out once more during teardown; their
// stale rectangles must not overwrite the current screen's.
func newChromeControlSet() uint64 {
	chromeGeomMu.Lock()
	defer chromeGeomMu.Unlock()
	chromeGeom.generation++
	chromeGeom.min = chromeRect{}
	chromeGeom.max = chromeRect{}
	chromeGeom.close = chromeRect{}
	chromeGeom.sidebar = chromeRect{}
	chromeGeom.hasSidebar = false
	return chromeGeom.generation
}

func updateChromeGeometry(setID uint64, c fyne.Canvas, glyph chromeGlyph, rect chromeRect) {
	chromeGeomMu.Lock()
	defer chromeGeomMu.Unlock()
	if setID != chromeGeom.generation {
		return
	}
	chromeGeom.scale = c.Scale()
	chromeGeom.canvasW = c.Size().Width
	chromeGeom.canvasH = c.Size().Height
	switch glyph {
	case glyphMinimize:
		chromeGeom.min = rect
	case glyphMaximize, glyphRestore:
		chromeGeom.max = rect
	case glyphClose:
		chromeGeom.close = rect
	case glyphSidebar:
		chromeGeom.sidebar = rect
		chromeGeom.hasSidebar = true
	}
}

func clearChromeSidebar() {
	chromeGeomMu.Lock()
	chromeGeom.hasSidebar = false
	chromeGeomMu.Unlock()
}

func chromeGeometrySnapshot() chromeGeometry {
	chromeGeomMu.Lock()
	defer chromeGeomMu.Unlock()
	return chromeGeom
}

type chromeGlyph int

const (
	glyphMinimize chromeGlyph = iota
	glyphMaximize
	glyphRestore
	glyphClose
	glyphSidebar
)

// chromeControl identifies the control under the cursor for the native hover
// hook. Sidebar is included for completeness; it is hit-tested as client area,
// so Fyne reports its hover itself.
type chromeControl int

const (
	controlNone chromeControl = iota
	controlMinimize
	controlMaximize
	controlClose
	controlSidebar
)

// chromeButton is a window control that draws its own rounded hover surface
// and vector glyph. Windows owns the minimize/maximize/close hit regions, so
// hover is driven from WM_NCMOUSEMOVE rather than Fyne mouse events.
type chromeButton struct {
	widget.BaseWidget

	glyph        chromeGlyph
	setID        uint64
	danger       bool
	active       bool
	hovered      bool
	glyphVisible bool
	mirrored     bool
	onTapped     func()
	onHover      func(bool)
}

func newChromeButton(glyph chromeGlyph, action func()) *chromeButton {
	b := &chromeButton{glyph: glyph, onTapped: action, glyphVisible: true}
	b.ExtendBaseWidget(b)
	return b
}

func (b *chromeButton) MinSize() fyne.Size {
	return fyne.NewSize(chromeButtonWidth, chromeButtonHeight)
}

// Move updates the native hit region whenever the surrounding layout shifts the
// control. A move alone does not re-run the renderer's Layout, so tracking it
// here is what keeps the regions aligned after the window is resized.
func (b *chromeButton) Move(pos fyne.Position) {
	b.BaseWidget.Move(pos)
	b.updateGeometry()
}

func (b *chromeButton) updateGeometry() {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	c := app.Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	p := app.Driver().AbsolutePositionForObject(b)
	updateChromeGeometry(b.setID, c, b.glyph, chromeRect{
		X: p.X, Y: p.Y, W: b.Size().Width, H: b.Size().Height,
	})
}

func (b *chromeButton) Tapped(*fyne.PointEvent) {
	if b.onTapped != nil {
		b.onTapped()
	}
}

func (b *chromeButton) MouseIn(*desktop.MouseEvent)    { b.SetHovered(true) }
func (b *chromeButton) MouseMoved(*desktop.MouseEvent) {}
func (b *chromeButton) MouseOut()                      { b.SetHovered(false) }

func (b *chromeButton) SetHovered(v bool) {
	if b.hovered == v {
		return
	}
	b.hovered = v
	if b.onHover != nil {
		b.onHover(v)
	}
	b.Refresh()
}

func (b *chromeButton) SetGlyphVisible(v bool) {
	if b.glyphVisible == v {
		return
	}
	b.glyphVisible = v
	b.Refresh()
}

// SetMirrored flips the sidebar chevron: left = collapse, right = expand.
func (b *chromeButton) SetMirrored(v bool) {
	if b.mirrored == v {
		return
	}
	b.mirrored = v
	b.Refresh()
}

func (b *chromeButton) SetActive(v bool) {
	if b.active == v {
		return
	}
	b.active = v
	b.Refresh()
}

func (b *chromeButton) SetGlyph(g chromeGlyph) {
	if b.glyph == g {
		return
	}
	b.glyph = g
	b.Refresh()
}

type chromeButtonRenderer struct {
	btn *chromeButton
	bg  *canvas.Rectangle

	minLine *canvas.Line

	closeA, closeB *canvas.Line

	maxRect                   *canvas.Rectangle
	restoreBack, restoreFront *canvas.Rectangle

	sidebarRect        *canvas.Rectangle
	sidebarDivider     *canvas.Line
	chevronA, chevronB *canvas.Line

	lines   []*canvas.Line
	rects   []*canvas.Rectangle
	objects []fyne.CanvasObject
}

func (b *chromeButton) CreateRenderer() fyne.WidgetRenderer {
	r := &chromeButtonRenderer{btn: b}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 9

	addLine := func() *canvas.Line {
		l := canvas.NewLine(color.Transparent)
		l.StrokeWidth = 1.3
		r.lines = append(r.lines, l)
		r.objects = append(r.objects, l)
		return l
	}
	addRect := func(radius float32) *canvas.Rectangle {
		rect := canvas.NewRectangle(color.Transparent)
		rect.StrokeWidth = 1.3
		rect.CornerRadius = radius
		r.rects = append(r.rects, rect)
		r.objects = append(r.objects, rect)
		return rect
	}

	switch b.glyph {
	case glyphMinimize:
		r.minLine = addLine()
	case glyphClose:
		r.closeA = addLine()
		r.closeB = addLine()
	case glyphMaximize, glyphRestore:
		r.maxRect = addRect(2)
		r.restoreBack = addRect(2)
		r.restoreFront = addRect(2)
	case glyphSidebar:
		r.sidebarRect = addRect(3)
		r.sidebarDivider = addLine()
		r.chevronA = addLine()
		r.chevronB = addLine()
	}

	// Background first, then the glyph strokes.
	r.objects = append([]fyne.CanvasObject{r.bg}, r.objects...)
	r.Refresh()
	return r
}

func (r *chromeButtonRenderer) MinSize() fyne.Size { return r.btn.MinSize() }

func (r *chromeButtonRenderer) Layout(size fyne.Size) {
	// Keep the native hit test pointing at the controls' real positions on
	// resize and monitor changes.
	r.btn.updateGeometry()

	r.bg.Move(fyne.NewPos(0, 0))
	r.bg.Resize(size)

	cx, cy := size.Width/2, size.Height/2
	switch r.btn.glyph {
	case glyphMinimize:
		setLine(r.minLine, cx-6, cy, cx+6, cy)
	case glyphClose:
		setLine(r.closeA, cx-5, cy-5, cx+5, cy+5)
		setLine(r.closeB, cx+5, cy-5, cx-5, cy+5)
	case glyphMaximize, glyphRestore:
		setRect(r.maxRect, cx-5.5, cy-5.5, 11, 11)
		setRect(r.restoreBack, cx-3.5, cy-6.5, 9, 9)
		setRect(r.restoreFront, cx-5.5, cy-4.5, 9, 9)
	case glyphSidebar:
		setRect(r.sidebarRect, cx-8, cy-7, 16, 14)
		setLine(r.sidebarDivider, cx+3, cy-6, cx+3, cy+6)
		if r.btn.mirrored {
			setLine(r.chevronA, cx-1, cy-3, cx-4, cy)
			setLine(r.chevronB, cx-4, cy, cx-1, cy+3)
		} else {
			setLine(r.chevronA, cx-4, cy-3, cx-1, cy)
			setLine(r.chevronB, cx-1, cy, cx-4, cy+3)
		}
	}

	r.apply()
}

func (r *chromeButtonRenderer) Refresh() {
	r.apply()
	for _, o := range r.objects {
		o.Refresh()
	}
}

func (r *chromeButtonRenderer) Objects() []fyne.CanvasObject { return r.objects }

func (r *chromeButtonRenderer) Destroy() {}

func (r *chromeButtonRenderer) apply() {
	b := r.btn

	fill := color.Color(color.Transparent)
	switch {
	case b.danger && b.hovered:
		fill = colorDanger
	case b.hovered:
		fill = colorHover
	case b.active:
		fill = colorPressed
	}
	r.bg.FillColor = fill

	fg := color.Color(colorText)
	if b.danger && b.hovered {
		fg = colorWhite
	}
	for _, l := range r.lines {
		l.StrokeColor = fg
		setVisible(l, b.glyphVisible)
	}
	for _, rect := range r.rects {
		rect.StrokeColor = fg
		setVisible(rect, b.glyphVisible)
	}
	if !b.glyphVisible {
		return
	}

	// Maximize and restore share a renderer; show the group the glyph asks for.
	restore := b.glyph == glyphRestore
	if r.maxRect != nil {
		setVisible(r.maxRect, !restore)
		setVisible(r.restoreBack, restore)
		setVisible(r.restoreFront, restore)
	}
}

func setVisible(o fyne.CanvasObject, v bool) {
	if v {
		o.Show()
		return
	}
	o.Hide()
}

func setLine(l *canvas.Line, x1, y1, x2, y2 float32) {
	l.Position1 = fyne.NewPos(x1, y1)
	l.Position2 = fyne.NewPos(x2, y2)
}

func setRect(r *canvas.Rectangle, x, y, w, h float32) {
	r.Move(fyne.NewPos(x, y))
	r.Resize(fyne.NewSize(w, h))
}
