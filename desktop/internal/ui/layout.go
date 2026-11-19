package ui

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// The layout primitives every screen shares. Each one exists because the
// Fyne default it replaces produced a visible defect: grids spaced by the
// 5px theme padding, split dividers drawn as a stray bar half way down the
// pane, scrollbars painted over the right edge of a card, and chips that sat
// above the text of the label beside them.

// scrollGutter is the strip a scrolling page keeps free on its right for the
// scrollbar, so the bar never covers a card's edge.
const scrollGutter = spaceM

// ---- line heights -----------------------------------------------------------

var (
	lineOnce   sync.Once
	lineHeight float32 // a single-line widget.Label
)

func measureLines() {
	lineOnce.Do(func() {
		lineHeight = widget.NewLabel("Ag").MinSize().Height
	})
}

// rowLineHeight is the height of a one-line label: the line every row's
// content is centred on.
func rowLineHeight() float32 {
	measureLines()
	return lineHeight
}

// lineLayout gives its child at least height h and centres it vertically,
// keeping the child's own width.
type lineLayout struct{ h float32 }

func (l lineLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var s fyne.Size
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		s = s.Max(o.MinSize())
	}
	if s.Height < l.h {
		s.Height = l.h
	}
	return s
}

func (l lineLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		m := o.MinSize()
		o.Resize(fyne.NewSize(size.Width, m.Height))
		o.Move(fyne.NewPos(0, (size.Height-m.Height)/2))
	}
}

// onLine makes a compact object (a chip, a dot, a keycap) as tall as a label
// and centres it, so it sits on the same line as the text beside it.
func onLine(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(lineLayout{h: rowLineHeight()}, o)
}

// headerLine reserves a button's height for a header row, so two panels side
// by side keep their titles level whether or not one has an action.
func headerLine(o fyne.CanvasObject) fyne.CanvasObject {
	return container.New(lineLayout{h: controlHeight}, o)
}

// ---- grid -------------------------------------------------------------------

// gapGrid is a column grid with real gaps. Fyne's grid uses the theme padding
// (5px) between cells, which reads as cards touching.
type gapGrid struct {
	cols           int
	colGap, rowGap float32
}

func (g gapGrid) visible(objs []fyne.CanvasObject) []fyne.CanvasObject {
	out := objs[:0:0]
	for _, o := range objs {
		if o.Visible() {
			out = append(out, o)
		}
	}
	return out
}

func (g gapGrid) MinSize(objs []fyne.CanvasObject) fyne.Size {
	objs = g.visible(objs)
	if len(objs) == 0 {
		return fyne.Size{}
	}
	var cell fyne.Size
	for _, o := range objs {
		cell = cell.Max(o.MinSize())
	}
	cols := g.cols
	if len(objs) < cols {
		cols = len(objs)
	}
	rows := (len(objs) + g.cols - 1) / g.cols
	h := float32(0)
	for r := 0; r < rows; r++ {
		h += g.rowHeight(objs, r)
	}
	return fyne.NewSize(cell.Width*float32(g.cols)+g.colGap*float32(g.cols-1), h+g.rowGap*float32(rows-1))
}

// rowHeight is the tallest cell in row r: rows size to their content, so a
// short row of fields does not inherit a tall card's height.
func (g gapGrid) rowHeight(objs []fyne.CanvasObject, r int) float32 {
	h := float32(0)
	for c := 0; c < g.cols; c++ {
		i := r*g.cols + c
		if i >= len(objs) {
			break
		}
		if mh := objs[i].MinSize().Height; mh > h {
			h = mh
		}
	}
	return h
}

func (g gapGrid) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	objs = g.visible(objs)
	if len(objs) == 0 {
		return
	}
	w := (size.Width - g.colGap*float32(g.cols-1)) / float32(g.cols)
	y := float32(0)
	rows := (len(objs) + g.cols - 1) / g.cols
	for r := 0; r < rows; r++ {
		h := g.rowHeight(objs, r)
		for c := 0; c < g.cols; c++ {
			i := r*g.cols + c
			if i >= len(objs) {
				break
			}
			objs[i].Move(fyne.NewPos(float32(c)*(w+g.colGap), y))
			objs[i].Resize(fyne.NewSize(w, h))
		}
		y += h + g.rowGap
	}
}

// grid lays objects out in equal columns with the standard gap.
func grid(cols int, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(gapGrid{cols: cols, colGap: spaceM, rowGap: spaceM}, objs...)
}

// checkColumns is a grid for checkboxes: columns apart, rows tight.
func checkColumns(cols int, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(gapGrid{cols: cols, colGap: spaceM, rowGap: 0}, objs...)
}

// ---- detail rows ------------------------------------------------------------

// detailLabelWidth is the label column of every card's key/value rows.
const detailLabelWidth = 112

// detailRow is one key/value line in a card. The label and the value share a
// size and a line, so their text sits on one baseline.
func detailRow(label string, value fyne.CanvasObject) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widthBox(detailLabelWidth, onLine(fieldLabel(label))), nil, onLine(value))
}

// detailText is a detail row's value: one size and face for every card.
func detailText(text string) *widget.Label {
	return monoInline(orDash(text))
}

// ---- page scroll ------------------------------------------------------------

// pageScroll is a scrolling page body. The content keeps a gutter on its right
// for the scrollbar and air at the bottom, so the last card is not clipped.
// Pair it with scrollInset, which narrows the page's right margin by the same
// gutter so the content's edge still lines up with the header above it.
func pageScroll(content fyne.CanvasObject) *container.Scroll {
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, spaceL, 0, scrollGutter), content))
}

// scrollInset is pageInset for a body that scrolls with pageScroll.
func scrollInset(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(layout.NewCustomPaddedLayout(pageTop, 0, pageGutter, pageGutter-scrollGutter), obj)
}

// paneScroll is a scrolling pane inside a split: the same gutter, no page
// margins.
func paneScroll(content fyne.CanvasObject) *container.Scroll {
	return container.NewVScroll(container.New(layout.NewCustomPaddedLayout(0, spaceS, 0, scrollGutter), content))
}

// ---- split pane -------------------------------------------------------------

// splitPane is a resizable two-pane container with a hairline divider that
// runs the full length of the panes. Fyne's split draws its grip as a short
// bright bar in the middle of the divider, which read as a stray mark.
type splitPane struct {
	horizontal bool
	offset     float32
	box        *fyne.Container
	divider    *paneDivider
}

// Lead and trail are the panes; offset is the leading pane's share (0..1).
func newHSplitPane(lead, trail fyne.CanvasObject, offset float32) *fyne.Container {
	return newSplitPane(true, lead, trail, offset)
}

func newVSplitPane(lead, trail fyne.CanvasObject, offset float32) *fyne.Container {
	return newSplitPane(false, lead, trail, offset)
}

func newSplitPane(horizontal bool, lead, trail fyne.CanvasObject, offset float32) *fyne.Container {
	s := &splitPane{horizontal: horizontal, offset: offset}
	s.divider = newPaneDivider(s)
	s.box = container.New(s, lead, s.divider, trail)
	return s.box
}

// dividerHit is the divider's grab width; the line itself is one pixel.
const dividerHit = spaceS + 1

func (s *splitPane) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) < 3 {
		return fyne.Size{}
	}
	a, b := objs[0].MinSize(), objs[2].MinSize()
	if s.horizontal {
		return fyne.NewSize(a.Width+dividerHit+b.Width, fyne.Max(a.Height, b.Height))
	}
	return fyne.NewSize(fyne.Max(a.Width, b.Width), a.Height+dividerHit+b.Height)
}

func (s *splitPane) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) < 3 {
		return
	}
	lead, div, trail := objs[0], objs[1], objs[2]
	total := size.Width
	if !s.horizontal {
		total = size.Height
	}
	avail := total - dividerHit
	minLead, minTrail := lead.MinSize().Width, trail.MinSize().Width
	if !s.horizontal {
		minLead, minTrail = lead.MinSize().Height, trail.MinSize().Height
	}
	at := avail * s.offset
	if at < minLead {
		at = minLead
	}
	if at > avail-minTrail {
		at = avail - minTrail
	}
	if at < 0 {
		at = 0
	}
	if s.horizontal {
		lead.Move(fyne.NewPos(0, 0))
		lead.Resize(fyne.NewSize(at, size.Height))
		div.Move(fyne.NewPos(at, 0))
		div.Resize(fyne.NewSize(dividerHit, size.Height))
		trail.Move(fyne.NewPos(at+dividerHit, 0))
		trail.Resize(fyne.NewSize(avail-at, size.Height))
		return
	}
	lead.Move(fyne.NewPos(0, 0))
	lead.Resize(fyne.NewSize(size.Width, at))
	div.Move(fyne.NewPos(0, at))
	div.Resize(fyne.NewSize(size.Width, dividerHit))
	trail.Move(fyne.NewPos(0, at+dividerHit))
	trail.Resize(fyne.NewSize(size.Width, avail-at))
}

// paneDivider is the grab strip between the panes: a centred hairline in the
// border colour, the accent while hovered or dragged.
type paneDivider struct {
	widget.BaseWidget
	split   *splitPane
	line    *canvas.Rectangle
	hovered bool
	drag    float32
}

func newPaneDivider(s *splitPane) *paneDivider {
	d := &paneDivider{split: s, line: canvas.NewRectangle(colorBorder)}
	d.ExtendBaseWidget(d)
	return d
}

func (d *paneDivider) CreateRenderer() fyne.WidgetRenderer {
	return &paneDividerRenderer{d: d}
}

func (d *paneDivider) lineColor() color.Color {
	if d.hovered {
		return colorAccent
	}
	return colorBorder
}

func (d *paneDivider) Cursor() desktop.Cursor {
	if d.split.horizontal {
		return desktop.HResizeCursor
	}
	return desktop.VResizeCursor
}

func (d *paneDivider) MouseIn(*desktop.MouseEvent) {
	d.hovered = true
	d.Refresh()
}

func (d *paneDivider) MouseMoved(*desktop.MouseEvent) {}

func (d *paneDivider) MouseOut() {
	d.hovered = false
	d.Refresh()
}

func (d *paneDivider) Dragged(e *fyne.DragEvent) {
	box := d.split.box
	total := box.Size().Width
	delta := e.Dragged.DX
	if !d.split.horizontal {
		total = box.Size().Height
		delta = e.Dragged.DY
	}
	avail := total - dividerHit
	if avail <= 0 {
		return
	}
	d.split.offset += delta / avail
	if d.split.offset < 0.05 {
		d.split.offset = 0.05
	}
	if d.split.offset > 0.95 {
		d.split.offset = 0.95
	}
	box.Refresh()
}

func (d *paneDivider) DragEnd() {}

type paneDividerRenderer struct{ d *paneDivider }

func (r *paneDividerRenderer) Layout(size fyne.Size) {
	if r.d.split.horizontal {
		r.d.line.Move(fyne.NewPos((size.Width-1)/2, 0))
		r.d.line.Resize(fyne.NewSize(1, size.Height))
		return
	}
	r.d.line.Move(fyne.NewPos(0, (size.Height-1)/2))
	r.d.line.Resize(fyne.NewSize(size.Width, 1))
}

func (r *paneDividerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(dividerHit, dividerHit)
}

func (r *paneDividerRenderer) Refresh() {
	r.d.line.FillColor = r.d.lineColor()
	r.d.line.Refresh()
	r.Layout(r.d.Size())
}

func (r *paneDividerRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.d.line} }
func (r *paneDividerRenderer) Destroy()                     {}
