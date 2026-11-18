package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// railRow is the sidebar's navigation item: icon, label and a hover/selected
// surface. Shortcuts are listed in the command palette rather than on every
// row. Rows are 28px tall so every destination fits the default window.
type railRow struct {
	widget.BaseWidget

	bg      *canvas.Rectangle
	label   *widget.Label
	stack   *fyne.Container
	action  func()
	active  bool
	hovered bool
}

func newRailRow(label string, icon fyne.Resource, shortcut string, action func()) *railRow {
	r := &railRow{action: action}
	r.bg = canvas.NewRectangle(color.Transparent)
	r.bg.CornerRadius = 5

	iconObj := container.NewGridWrap(fyne.NewSize(15, 15), widget.NewIcon(icon))
	r.label = widget.NewLabel(label)
	r.label.SizeName = theme.SizeNameText
	r.label.Truncation = fyne.TextTruncateEllipsis

	left := container.New(layout.NewCustomPaddedLayout(0, 0, spaceS+2, 0), container.NewCenter(iconObj))
	right := hSpacer(spaceS)
	text := container.New(compactLabelLayout{}, r.label)
	row := container.NewBorder(nil, nil, left, right,
		container.New(layout.NewCustomPaddedLayout(0, 0, 2, 0), vLeft(text)))
	r.stack = container.NewStack(r.bg, row)
	r.ExtendBaseWidget(r)
	r.apply()
	return r
}

// railRowHeight is the pitch of one destination in the rail.
const railRowHeight = float32(28)

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
	case r.active, r.hovered:
		fill = colorHover
	}
	r.bg.FillColor = fill
	r.bg.Refresh()
	if r.label != nil {
		if r.active {
			r.label.Importance = widget.MediumImportance
		} else {
			r.label.Importance = widget.LowImportance
		}
		r.label.Refresh()
	}
}

func railSection(text string) fyne.CanvasObject {
	t := canvas.NewText(text, colorFaint)
	t.TextSize = 11
	return container.New(layout.NewCustomPaddedLayout(spaceL, spaceXS, spaceM, 0), t)
}
