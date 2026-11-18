package ui

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// A hint shows its text while the pointer is over the glyph and removes it when
// the pointer leaves; the tooltip stays inside the window; and, because the
// layer it is drawn on takes no input, a button the tooltip covers can still
// be pressed.
func TestHintShowsAndHidesItsTooltip(t *testing.T) {
	app := test.NewApp()
	claimUI(t)
	defer app.Quit()
	defer func() { tipLayer = nil }()
	win := app.NewWindow("hint")

	hint := newHint("Entry points that route messages into agents. Saves go to the deployment config and bind at the next runtime start.")
	pressed := 0
	under := widget.NewButton("under the tooltip", func() { pressed++ })
	page := container.NewVBox(container.NewHBox(widget.NewLabel("Channels"), hint), under)
	win.SetContent(container.NewStack(canvas.NewRectangle(colorBG), page, newTipLayer()))
	win.Resize(fyne.NewSize(420, 220))

	at := fyne.CurrentApp().Driver().AbsolutePositionForObject(hint)
	test.MoveMouse(win.Canvas(), at.Add(fyne.NewPos(hint.Size().Width/2, hint.Size().Height/2)))
	if len(tipLayer.Objects) != 1 {
		t.Fatalf("tooltip layer holds %d objects while hovering, want 1", len(tipLayer.Objects))
	}
	tip := tipLayer.Objects[0]
	if tip.Position().X < 0 || tip.Position().X+tip.Size().Width > win.Canvas().Size().Width {
		t.Errorf("tooltip at x=%v width %v leaves a %v-wide window", tip.Position().X, tip.Size().Width, win.Canvas().Size().Width)
	}
	if tip.Size().Width > tipMaxWidth+8 || tip.Size().Height < 30 {
		t.Errorf("tooltip size %v: want the text wrapped within %v", tip.Size(), tipMaxWidth)
	}
	if dir := os.Getenv("STUDIO_SNAPSHOT_DIR"); dir != "" {
		if f, err := os.Create(filepath.Join(dir, "18-hint.png")); err == nil {
			_ = png.Encode(f, win.Canvas().Capture())
			f.Close()
		}
	}

	// The button sits under the tooltip; a press reaches it.
	bp := fyne.CurrentApp().Driver().AbsolutePositionForObject(under)
	test.TapCanvas(win.Canvas(), bp.Add(fyne.NewPos(10, under.Size().Height/2)))
	if pressed != 1 {
		t.Errorf("a button under the tooltip got %d presses, want 1", pressed)
	}

	test.MoveMouse(win.Canvas(), fyne.NewPos(400, 210))
	if len(tipLayer.Objects) != 0 {
		t.Fatalf("tooltip still showing after the pointer left (%d objects)", len(tipLayer.Objects))
	}
}

// A screen change removes a tooltip whose glyph went away with the screen.
func TestClearTipsDropsAStaleTooltip(t *testing.T) {
	defer func() { tipLayer = nil }()
	clearTips() // no layer yet: nothing to do, and no panic
	newTipLayer()
	tipLayer.Add(canvas.NewRectangle(colorBG))
	clearTips()
	if len(tipLayer.Objects) != 0 {
		t.Fatal("clearTips left a tooltip behind")
	}
}
