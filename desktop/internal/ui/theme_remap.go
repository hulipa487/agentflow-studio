package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// remapThemeColors updates colors that were captured in canvas objects when a
// screen was built. Widgets read the theme themselves; custom chrome widgets
// re-apply their palette on Refresh. This lets the theme and palette switch in
// place without discarding form state, scroll positions or unsaved edits.
func remapThemeColors(root fyne.CanvasObject, old, next palette) {
	if root == nil {
		return
	}
	m := map[color.NRGBA]color.NRGBA{
		old.BG: next.BG, old.Surface: next.Surface, old.Surface2: next.Surface2,
		old.Border: next.Border, old.Text: next.Text, old.Muted: next.Muted,
		old.Faint: next.Faint, old.Accent: next.Accent, old.Accent2: next.Accent2,
		old.AccentDim: next.AccentDim, old.Success: next.Success,
		old.Warning: next.Warning, old.Danger: next.Danger,
		old.Hover: next.Hover, old.Pressed: next.Pressed,
		old.White: next.White, old.Float: next.Float, old.OnPrimary: next.OnPrimary,
		old.ToneAccent: next.ToneAccent, old.ToneSuccess: next.ToneSuccess,
		old.ToneWarning: next.ToneWarning, old.ToneDanger: next.ToneDanger,
	}
	remapObject(root, m)
}

func remapObject(o fyne.CanvasObject, m map[color.NRGBA]color.NRGBA) {
	if o == nil {
		return
	}
	switch v := o.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			remapObject(child, m)
		}
		v.Refresh()
		return
	case *banner:
		remapObject(v.Container, m)
		return
	case *container.Scroll:
		remapObject(v.Content, m)
		v.Refresh()
		return
	case *container.Split:
		remapObject(v.Leading, m)
		remapObject(v.Trailing, m)
		v.Refresh()
		return
	case *container.AppTabs:
		for _, item := range v.Items {
			if item != nil {
				remapObject(item.Content, m)
			}
		}
		v.Refresh()
		return
	case *chromeButton:
		v.Refresh()
		return
	case *railRow:
		v.Refresh()
		return
	case *transStripe:
		v.Refresh()
		return
	case *canvas.Rectangle:
		v.FillColor = remapColor(v.FillColor, m)
		v.StrokeColor = remapColor(v.StrokeColor, m)
	case *canvas.Text:
		v.Color = remapColor(v.Color, m)
	case *canvas.Circle:
		v.FillColor = remapColor(v.FillColor, m)
		v.StrokeColor = remapColor(v.StrokeColor, m)
	case *canvas.Line:
		v.StrokeColor = remapColor(v.StrokeColor, m)
	case *canvas.LinearGradient:
		v.StartColor = remapColor(v.StartColor, m)
		v.EndColor = remapColor(v.EndColor, m)
	case *canvas.Image:
		if v.Resource != nil && strings.HasPrefix(v.Resource.Name(), "agentflow-logo") {
			v.Resource = logoResource()
		}
	}
	o.Refresh()
}

func remapColor(c color.Color, m map[color.NRGBA]color.NRGBA) color.Color {
	if c == nil {
		return c
	}
	n, ok := c.(color.NRGBA)
	if !ok {
		n = color.NRGBAModel.Convert(c).(color.NRGBA)
	}
	if next, ok := m[n]; ok {
		return next
	}
	return c
}
