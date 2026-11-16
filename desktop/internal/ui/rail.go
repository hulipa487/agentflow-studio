package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// railRow is the sidebar's navigation item: active indicator bar, icon, label,
// shortcut pill and a responsive hover/selected surface.
type railRow struct {
	widget.BaseWidget

	bg       *canvas.Rectangle
	bar      *canvas.Rectangle
	label    *widget.Label
	shortcut *widget.Label
	stack    *fyne.Container
	action   func()
	active   bool
	hovered  bool
}

func newRailRow(label string, icon fyne.Resource, shortcut string, action func()) *railRow {
	r := &railRow{action: action}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 8

	r.bar = canvas.NewRectangle(color.Transparent)
	r.bar.CornerRadius = 2
	r.bar.SetMinSize(fyne.NewSize(3, 18))

	iconObj := widget.NewIcon(icon)
	r.label = widget.NewLabel(label)
	r.label.SizeName = theme.SizeNameText
	r.shortcut = caption(shortcut)
	r.shortcut.Alignment = fyne.TextAlignTrailing
	r.shortcut.Wrapping = fyne.TextWrapOff
	r.shortcut.Truncation = fyne.TextTruncateClip

	barWrap := container.New(layout.NewCustomPaddedLayout(6, 6, 2, 0), r.bar)
	left := container.NewHBox(
		barWrap,
		container.New(layout.NewCustomPaddedLayout(0, 0, spaceXS, 0), container.NewCenter(iconObj)),
	)
	var right fyne.CanvasObject = hSpacer(spaceS)
	if shortcut != "" {
		right = container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), container.NewCenter(widthBox(58, r.shortcut)))
	}
	text := container.New(layout.NewCustomPaddedLayout(0, 0, spaceXS, 0), r.label)
	row := container.NewBorder(nil, nil, left, right, text)
	inner := container.New(layout.NewCustomPaddedLayout(3, 3, 0, 0), row)
	r.stack = container.NewStack(r.bg, inner)
	r.ExtendBaseWidget(r)
	r.apply()
	return r
}

func (r *railRow) MinSize() fyne.Size {
	min := r.stack.MinSize()
	if min.Height < 34 {
		min.Height = 34
	}
	return min
}

func (r *railRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.stack)
}

func (r *railRow) Refresh() {
	r.apply()
	r.BaseWidget.Refresh()
}

func (r *railRow) Tapped(*fyne.PointEvent) {
	if r.action != nil {
		r.action()
	}
}

func (r *railRow) MouseIn(*desktop.MouseEvent) {
	r.hovered = true
	r.apply()
}

func (r *railRow) MouseMoved(*desktop.MouseEvent) {}

func (r *railRow) MouseOut() {
	r.hovered = false
	r.apply()
}

func (r *railRow) SetActive(active bool) {
	if r.active == active {
		return
	}
	r.active = active
	r.apply()
}

func (r *railRow) apply() {
	var fill color.Color = color.Transparent
	var barFill color.Color = color.Transparent
	var stroke color.Color = color.Transparent
	switch {
	case r.active && r.hovered:
		fill = colorPressed
		barFill = colorAccent
		stroke = colorBorder
	case r.active:
		fill = colorSurface2
		barFill = colorAccent
		stroke = colorBorder
	case r.hovered:
		fill = colorHover
	}
	r.bg.FillColor = fill
	r.bg.StrokeColor = stroke
	r.bg.StrokeWidth = 1
	r.bg.Refresh()
	if r.bar != nil {
		r.bar.FillColor = barFill
		r.bar.Refresh()
	}
	if r.label != nil {
		r.label.TextStyle = fyne.TextStyle{Bold: r.active}
		r.label.Refresh()
	}
}

func railSection(text string) fyne.CanvasObject {
	l := caption(strings.ToUpper(text))
	l.TextStyle = fyne.TextStyle{Bold: true}
	return container.New(layout.NewCustomPaddedLayout(spaceS, spaceXS, spaceS+4, 0), l)
}
