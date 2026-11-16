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

// railRow is the sidebar's navigation item: a blue-to-pink indicator bar when
// active, icon, label, a shortcut keycap and a hover/selected surface. Rows are
// 30px tall so every destination fits the default window without scrolling.
type railRow struct {
	widget.BaseWidget

	bg       *canvas.Rectangle
	bar      *canvas.LinearGradient
	label    *widget.Label
	shortcut fyne.CanvasObject
	stack    *fyne.Container
	action   func()
	active   bool
	hovered  bool
}

func newRailRow(label string, icon fyne.Resource, shortcut string, action func()) *railRow {
	r := &railRow{action: action}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 7

	r.bar = accentBar(3, 16)
	r.bar.Hide()

	iconObj := container.NewGridWrap(fyne.NewSize(15, 15), widget.NewIcon(icon))
	r.label = widget.NewLabel(label)
	r.label.SizeName = theme.SizeNameText
	r.label.Truncation = fyne.TextTruncateEllipsis

	barWrap := container.NewCenter(container.NewGridWrap(fyne.NewSize(3, 16), r.bar))
	left := container.NewHBox(
		barWrap,
		container.New(layout.NewCustomPaddedLayout(0, 0, spaceS, 0), container.NewCenter(iconObj)),
	)
	var right fyne.CanvasObject = hSpacer(spaceS)
	if shortcut != "" {
		r.shortcut = kbd(shortcut)
		right = container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), container.NewCenter(r.shortcut))
	}
	text := container.New(compactLabelLayout{}, r.label)
	row := container.NewBorder(nil, nil, left, right,
		container.New(layout.NewCustomPaddedLayout(0, 0, 2, 0), vLeft(text)))
	r.stack = container.NewStack(r.bg, row)
	r.ExtendBaseWidget(r)
	r.apply()
	return r
}

// railRowHeight is the pitch of one destination in the rail.
const railRowHeight = float32(30)

func (r *railRow) MinSize() fyne.Size {
	min := r.stack.MinSize()
	min.Height = railRowHeight
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
	switch {
	case r.active && r.hovered:
		fill = colorPressed
	case r.active:
		fill = colorSurface2
	case r.hovered:
		fill = colorHover
	}
	r.bg.FillColor = fill
	r.bg.Refresh()
	if r.bar != nil {
		r.bar.StartColor = colorAccent
		r.bar.EndColor = colorAccent2
		if r.active {
			r.bar.Show()
		} else {
			r.bar.Hide()
		}
		r.bar.Refresh()
	}
	if r.label != nil {
		r.label.TextStyle = fyne.TextStyle{Bold: r.active}
		r.label.Refresh()
	}
}

func railSection(text string) fyne.CanvasObject {
	t := canvas.NewText(strings.ToUpper(text), colorFaint)
	t.TextSize = 10
	t.TextStyle = fyne.TextStyle{Bold: true}
	return container.New(layout.NewCustomPaddedLayout(spaceM, spaceXS, spaceM, 0), t)
}
