package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Tooltips. Fyne has none of its own, and a pop-up will not do: while one is
// open it takes every pointer event in the window, so the pointer leaving the
// glyph that opened it would never be seen. Instead the shell keeps a layer
// above everything else with no layout and nothing interactive in it. A
// tooltip is drawn there, and since nothing in the layer answers to the
// pointer, the window underneath keeps working as if it were not there.

// tipLayer is the shell's tooltip layer; nil until the shell is built (and in
// tests that build a screen without one), in which case hints draw nothing.
var tipLayer *fyne.Container

// tipMaxWidth bounds a tooltip's text column.
const tipMaxWidth = float32(320)

// newTipLayer builds the layer and makes it the current one.
func newTipLayer() *fyne.Container {
	tipLayer = container.NewWithoutLayout()
	return tipLayer
}

// clearTips removes any tooltip still showing: one whose glyph disappeared
// with the screen it was on never hears the pointer leave.
func clearTips() {
	if tipLayer == nil || len(tipLayer.Objects) == 0 {
		return
	}
	tipLayer.Objects = nil
	tipLayer.Refresh()
}

// hintMark is the info glyph after a heading. Its text is the explanation a
// sentence under the heading used to hold: useful once, noise after that, so
// it is there on demand.
type hintMark struct {
	widget.BaseWidget
	text string
	icon *widget.Icon
	tip  fyne.CanvasObject
}

func newHint(text string) *hintMark {
	h := &hintMark{text: text, icon: widget.NewIcon(theme.InfoIcon())}
	h.ExtendBaseWidget(h)
	return h
}

func (h *hintMark) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewCenter(container.NewGridWrap(fyne.NewSize(14, 14), h.icon)))
}

func (h *hintMark) MinSize() fyne.Size { return fyne.NewSize(18, 18) }

func (h *hintMark) MouseIn(*desktop.MouseEvent) { h.show() }

func (h *hintMark) MouseMoved(*desktop.MouseEvent) {}

func (h *hintMark) MouseOut() { h.hide() }

func (h *hintMark) show() {
	if tipLayer == nil || h.text == "" {
		return
	}
	drv := fyne.CurrentApp().Driver()
	at := drv.AbsolutePositionForObject(h)
	clearTips()

	label := widget.NewLabel(h.text)
	label.Wrapping = fyne.TextWrapWord
	label.SizeName = theme.SizeNameText
	// The width is the text's own when it fits on one line, and the column
	// width otherwise; the height then follows from wrapping at that width.
	width := fyne.MeasureText(h.text, theme.TextSize(), fyne.TextStyle{}).Width + 2*theme.InnerPadding()
	if width > tipMaxWidth {
		width = tipMaxWidth
	}
	label.Resize(fyne.NewSize(width, 0))
	size := fyne.NewSize(width, label.MinSize().Height)
	label.Resize(size)

	bg := canvas.NewRectangle(colorFloat)
	bg.CornerRadius = 5
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(2, 2, 2, 2), label))
	cardSize := size.AddWidthHeight(4, 4)
	card.Resize(cardSize)

	pos := at.Add(fyne.NewPos(-8, h.Size().Height+6))
	// Keep it inside the window.
	if c := drv.CanvasForObject(h); c != nil {
		if over := pos.X + cardSize.Width - c.Size().Width + 8; over > 0 {
			pos.X -= over
		}
		if pos.X < 4 {
			pos.X = 4
		}
	}
	card.Move(pos)
	h.tip = card
	tipLayer.Objects = []fyne.CanvasObject{card}
	tipLayer.Refresh()
}

func (h *hintMark) hide() {
	if tipLayer == nil || h.tip == nil {
		return
	}
	for _, o := range tipLayer.Objects {
		if o == h.tip {
			clearTips()
			break
		}
	}
	h.tip = nil
}

// titleWithHint is a heading followed by its hint glyph, or the heading alone.
func titleWithHint(title fyne.CanvasObject, hint string) fyne.CanvasObject {
	if hint == "" {
		return title
	}
	return container.NewHBox(vCenter(title), vCenter(newHint(hint)))
}

var _ desktop.Hoverable = (*hintMark)(nil)
