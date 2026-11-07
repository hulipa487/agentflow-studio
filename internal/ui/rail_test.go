package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

func TestRailRowTapAndHover(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := app.NewWindow("rail")
	clicks := 0
	row := newRailRow("Agents", theme.AccountIcon(), "Ctrl+1", func() { clicks++ })
	win.SetContent(container.NewCenter(row))
	win.Resize(fyne.NewSize(300, 200))

	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(row)
	center := fyne.NewPos(pos.X+row.Size().Width/2, pos.Y+row.Size().Height/2)
	test.MoveMouse(win.Canvas(), center)
	t.Logf("row pos=%v size=%v hovered=%v", pos, row.Size(), row.hovered)
	test.TapCanvas(win.Canvas(), center)
	if clicks != 1 {
		t.Fatalf("rail row taps = %d, want 1", clicks)
	}
}
