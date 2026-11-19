package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// toggleButton is a button that stays pressed while what it opens is open:
// the builder's Run and Docs, which show and hide the dock. Off it reads as a
// normal outlined button; on it gets the raised fill and an accent edge.
type toggleButton struct {
	widget.BaseWidget
	btn    *widget.Button
	bg     *canvas.Rectangle
	active bool
}

func newToggleButton(label string, icon fyne.Resource, action func()) *toggleButton {
	t := &toggleButton{btn: widget.NewButtonWithIcon(label, icon, action)}
	t.btn.Importance = widget.LowImportance
	t.bg = canvas.NewRectangle(colorSurface)
	t.bg.CornerRadius = 5 // the theme's input radius, so it lines up with the buttons beside it
	t.bg.StrokeWidth = 1
	t.ExtendBaseWidget(t)
	t.apply()
	return t
}

func (t *toggleButton) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewStack(t.bg, t.btn))
}

func (t *toggleButton) setActive(on bool) {
	t.active = on
	t.apply()
}

func (t *toggleButton) apply() {
	if t.active {
		t.bg.FillColor = colorSurface2
		t.bg.StrokeColor = colorAccent
	} else {
		t.bg.FillColor = colorSurface
		t.bg.StrokeColor = colorBorder
	}
	t.bg.Refresh()
}

// Refresh re-reads the palette, so a theme switch recolours the frame.
func (t *toggleButton) Refresh() {
	t.apply()
	t.BaseWidget.Refresh()
}
