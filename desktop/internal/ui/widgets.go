package ui

import (
	"crypto/rand"
	"encoding/hex"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Spacing scale. Every page, card and toolbar uses these steps so gaps stay
// consistent across layouts.
const (
	spaceXS = 4
	spaceS  = 8
	spaceM  = 12
	spaceL  = 16
	spaceXL = 24
)

func vstack(gap float32, objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedVBoxLayout(gap), objects...)
}

func hstack(gap float32, objects ...fyne.CanvasObject) *fyne.Container {
	return container.New(layout.NewCustomPaddedHBoxLayout(gap), objects...)
}

func vCenter(o fyne.CanvasObject) fyne.CanvasObject {
	return container.NewCenter(o)
}

func hSpacer(width float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(width, 0))
	return r
}

func vSpacer(height float32) fyne.CanvasObject {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(0, height))
	return r
}

func monoEntry(multiline bool) *widget.Entry {
	var e *widget.Entry
	if multiline {
		e = widget.NewMultiLineEntry()
		e.Wrapping = fyne.TextWrapOff
	} else {
		e = widget.NewEntry()
	}
	e.TextStyle = fyne.TextStyle{Monospace: true}
	return e
}

func wrapLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

func clearBox(box *fyne.Container) {
	box.Objects = nil
	box.Refresh()
}

// widthBox constrains an object to a fixed width while letting it keep its
// natural height.
type widthLayout struct{ width float32 }

func (l widthLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		if m := o.MinSize(); m.Height > h {
			h = m.Height
		}
	}
	return fyne.NewSize(l.width, h)
}

func (l widthLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(l.width, o.MinSize().Height))
		o.Move(fyne.NewPos(0, 0))
	}
}

func widthBox(width float32, obj fyne.CanvasObject) fyne.CanvasObject {
	return container.New(widthLayout{width: width}, obj)
}

// searchBox is a modern, minimalist search field with a leading magnifier
// icon, a borderless inner text entry, and an auto-hiding clear button.
type searchBox struct {
	Entry *widget.Entry
	Root  fyne.CanvasObject
}

func newSearchBox(placeholder string, onChanged func(string)) *searchBox {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(placeholder)
	entry.SetMinRowsVisible(1)

	clearBtn := iconButton(theme.CancelIcon(), "Clear search", func() {
		entry.SetText("")
	})
	clearBtn.Hide()

	entry.OnChanged = func(q string) {
		if q == "" {
			clearBtn.Hide()
		} else {
			clearBtn.Show()
		}
		if onChanged != nil {
			onChanged(q)
		}
	}

	magIcon := widget.NewIcon(theme.SearchIcon())
	magWrap := container.New(layout.NewCustomPaddedLayout(0, 0, spaceS+2, 2),
		container.NewGridWrap(fyne.NewSize(15, 15), magIcon))
	clearWrap := container.New(layout.NewCustomPaddedLayout(0, 0, 0, 4), vCenter(clearBtn))

	cleanEntry := container.NewThemeOverride(entry, borderlessEntryTheme{})
	inner := container.NewBorder(nil, nil, vCenter(magWrap), clearWrap, container.New(singleLineEntryLayout{}, cleanEntry))

	bg := canvas.NewRectangle(colorBG)
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1

	root := controlFrame(container.NewStack(bg, inner))
	return &searchBox{Entry: entry, Root: root}
}

// showModalCard renders a minimalist custom modal popup card with a clean
// header, close glyph, content slot, and footer buttons.
func showModalCard(win fyne.Window, title, subtitle string, content fyne.CanvasObject, footer fyne.CanvasObject) *widget.PopUp {
	if win == nil || win.Canvas() == nil {
		return nil
	}
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	titleObj := insetLeading(titleText(title, theme.SizeNameHeadingText))
	closeBtn := iconButton(theme.CancelIcon(), "Close", closePopup)
	headRow := container.NewBorder(nil, nil, vCenter(titleObj), vCenter(closeBtn), nil)

	headBlock := vstack(spaceXS, headRow)
	if subtitle != "" {
		headBlock.Add(muted(subtitle))
	}

	items := []fyne.CanvasObject{headBlock, widget.NewSeparator(), content}
	if footer != nil {
		items = append(items, widget.NewSeparator(), footer)
	}
	body := vstack(spaceM, items...)

	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(18, 16, 20, 20), body))

	if c := win.Canvas(); c != nil && c.Content() != nil {
		c.Refresh(c.Content())
	}
	popup = widget.NewModalPopUp(card, win.Canvas())
	popup.Show()
	return popup
}

// showConfirmPrompt displays a modern, minimalist confirmation prompt modal.
func showConfirmPrompt(win fyne.Window, title, message, confirmLabel string, danger bool, onConfirm func(bool)) {
	if win == nil || win.Canvas() == nil {
		return
	}
	var popup *widget.PopUp
	closeWith := func(ok bool) {
		if popup != nil {
			popup.Hide()
		}
		if onConfirm != nil {
			onConfirm(ok)
		}
	}

	msgLabel := wrapLabel(message)
	content := widthBox(420, msgLabel)

	cancelBtn := widthBox(100, controlFrame(actionButton("Cancel", nil, func() { closeWith(false) })))
	confirmBtnWidget := primaryButton(confirmLabel, nil, func() { closeWith(true) })
	if danger {
		confirmBtnWidget.Importance = widget.DangerImportance
	}
	confirmBtn := widthBox(120, controlFrame(confirmBtnWidget))
	footer := container.NewBorder(nil, nil, nil, hstack(spaceS, cancelBtn, confirmBtn), nil)

	titleObj := insetLeading(titleText(title, theme.SizeNameSubHeadingText))
	closeBtn := iconButton(theme.CancelIcon(), "Close", func() { closeWith(false) })
	headRow := container.NewBorder(nil, nil, vCenter(titleObj), vCenter(closeBtn), nil)

	body := vstack(spaceM, headRow, widget.NewSeparator(), content, footer)
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(16, 16, 20, 20), body))

	if c := win.Canvas(); c != nil && c.Content() != nil {
		c.Refresh(c.Content())
	}
	popup = widget.NewModalPopUp(card, win.Canvas())
	popup.Show()
}

// showInfoPrompt displays a modern, minimalist information prompt modal.
func showInfoPrompt(win fyne.Window, title, message string) {
	if win == nil || win.Canvas() == nil {
		return
	}
	var popup *widget.PopUp
	closePopup := func() {
		if popup != nil {
			popup.Hide()
		}
	}

	msgLabel := wrapLabel(message)
	content := widthBox(420, msgLabel)
	okBtn := widthBox(100, controlFrame(primaryButton("Done", nil, closePopup)))
	footer := container.NewBorder(nil, nil, nil, okBtn, nil)

	titleObj := insetLeading(titleText(title, theme.SizeNameSubHeadingText))
	closeBtn := iconButton(theme.CancelIcon(), "Close", closePopup)
	headRow := container.NewBorder(nil, nil, vCenter(titleObj), vCenter(closeBtn), nil)

	body := vstack(spaceM, headRow, widget.NewSeparator(), content, footer)
	bg := canvas.NewRectangle(colorSurface)
	bg.CornerRadius = 12
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	card := container.NewStack(bg, container.New(layout.NewCustomPaddedLayout(16, 16, 20, 20), body))

	if c := win.Canvas(); c != nil && c.Content() != nil {
		c.Refresh(c.Content())
	}
	popup = widget.NewModalPopUp(card, win.Canvas())
	popup.Show()
}

func splitList(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func newThreadID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "thread"
	}
	return hex.EncodeToString(b[:])
}

func shortID(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}
