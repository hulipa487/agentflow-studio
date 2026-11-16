package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// resizeGrip replaces the resize edge that a frameless window loses. Dragging
// it lets the bottom-right corner follow the cursor.
type resizeGrip struct {
	widget.BaseWidget
	win fyne.Window
}

func newResizeGrip(win fyne.Window) *resizeGrip {
	g := &resizeGrip{win: win}
	g.ExtendBaseWidget(g)
	return g
}

type resizeGripRenderer struct {
	text *canvas.Text
}

func (g *resizeGrip) CreateRenderer() fyne.WidgetRenderer {
	t := canvas.NewText("◢", colorFaint)
	t.TextSize = 10
	t.Alignment = fyne.TextAlignCenter
	return &resizeGripRenderer{text: t}
}

func (r *resizeGripRenderer) Layout(size fyne.Size) {
	r.text.Resize(size)
	r.text.Move(fyne.NewPos(0, 0))
}

func (r *resizeGripRenderer) MinSize() fyne.Size { return fyne.NewSize(16, 16) }

func (r *resizeGripRenderer) Refresh() {
	r.text.Color = colorFaint
	r.text.Refresh()
}

func (r *resizeGripRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.text}
}

func (r *resizeGripRenderer) Destroy() {}

func (g *resizeGrip) MinSize() fyne.Size { return fyne.NewSize(16, 16) }

func (g *resizeGrip) Dragged(*fyne.DragEvent) { windowResizeToCursor(g.win.Title()) }

func (g *resizeGrip) DragEnd() {}

func (a *App) resizeGrip() fyne.CanvasObject {
	if !borderlessSupported() {
		return nil
	}
	return newResizeGrip(a.win)
}
