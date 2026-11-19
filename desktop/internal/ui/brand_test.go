package ui

import (
	"bytes"
	"image/png"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// The embedded brand files are the kit's: a real 1024px PNG icon and the
// hexagon mark SVG, which Fyne must be able to draw.
func TestBrandAssetsAreEmbedded(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(appIconResource().Content()))
	if err != nil {
		t.Fatalf("app icon is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("app icon is %dx%d, want 1024x1024", b.Dx(), b.Dy())
	}
	if !bytes.Contains(logoResource().Content(), []byte("af-univ-grad")) {
		t.Fatal("logo resource is not the brand kit's mark")
	}

	app := test.NewApp()
	claimUI(t)
	defer app.Quit()
	w := test.NewWindow(brandBanner(64, 32))
	w.Resize(fyne.NewSize(400, 120))
	shot := w.Canvas().Capture()
	// The mark must be painted as a hexagon, not flattened to a line: its
	// coloured pixels have to span most of its 64px height.
	top, bottom := -1, -1
	b := shot.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Min.X+90 && x < b.Max.X; x++ {
			r, g, bl, a := shot.At(x, y).RGBA()
			if a > 0 && (r != g || g != bl) {
				if top < 0 {
					top = y
				}
				bottom = y
				break
			}
		}
	}
	if top < 0 || bottom-top < 40 {
		t.Fatalf("the mark is %d px tall, want most of 64 (rows %d..%d)", bottom-top, top, bottom)
	}
	if !bytes.Contains(forFyne([]byte(`<g transform="translate(52, 32) scale(3.4)">`)), []byte("scale(3.4 3.4)")) {
		t.Fatal("scale(n) was not expanded for Fyne")
	}
}

// The wordmark recolours with the theme: the light theme's deeper gradient
// keeps "Flow" readable on a light surface.
func TestBrandWordmarkFollowsTheme(t *testing.T) {
	app := test.NewApp()
	claimUI(t)
	defer app.Quit()
	setTheme(paletteTrans, themeDark)
	defer setTheme(paletteTrans, themeDark)

	w := newBrandWordmark(30)
	dark := w.flow[0].Color
	if dark != brandGradient()[0] {
		t.Fatalf("F = %v, want the gradient's first stop %v", dark, brandGradient()[0])
	}
	if w.flow[3].Color != brandGradient()[2] {
		t.Fatalf("w = %v, want the gradient's last stop", w.flow[3].Color)
	}
	setThemePalette(themeLight)
	w.Refresh()
	if w.flow[0].Color == dark || w.flow[0].Color != brandGradient()[0] {
		t.Fatalf("after the switch F = %v, want the light gradient's %v", w.flow[0].Color, brandGradient()[0])
	}
	if w.agent.Color != colorText {
		t.Fatal("Agent should be in the text colour")
	}
}
