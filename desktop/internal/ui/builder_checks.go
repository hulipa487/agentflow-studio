package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// newChecks builds one checkbox per label with initial selection applied before
// callbacks are attached, so loading a definition never marks the builder dirty.
func newChecks(labels, selected []string) []*widget.Check {
	set := make(map[string]bool, len(selected))
	for _, s := range selected {
		set[s] = true
	}
	out := make([]*widget.Check, 0, len(labels))
	for _, label := range labels {
		c := widget.NewCheck(label, nil)
		c.SetChecked(set[label])
		out = append(out, c)
	}
	return out
}

func selectedChecks(checks []*widget.Check) []string {
	out := make([]string, 0, len(checks))
	for _, c := range checks {
		if c.Checked {
			out = append(out, c.Text)
		}
	}
	return out
}

func clearChecks(checks []*widget.Check) {
	for _, c := range checks {
		c.SetChecked(false)
	}
}

type compactCheckLayout struct{}

func (compactCheckLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if len(objs) == 0 {
		return fyne.Size{}
	}
	min := objs[0].MinSize()
	return fyne.NewSize(min.Width, 24)
}

func (compactCheckLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, obj := range objs {
		obj.Resize(fyne.NewSize(size.Width, size.Height))
		obj.Move(fyne.NewPos(0, 0))
	}
}

// compactCheck trims the default vertical padding of a checkbox while keeping
// its icon and text vertically centered.
func compactCheck(c *widget.Check) fyne.CanvasObject {
	return container.New(compactCheckLayout{}, c)
}

// checkGrid lays out checks in fixed columns with compact row height.
func checkGrid(checks []*widget.Check, columns int) fyne.CanvasObject {
	if columns < 1 {
		columns = 1
	}
	rows := vstack(2)
	for i := 0; i < len(checks); i += columns {
		row := make([]fyne.CanvasObject, 0, columns)
		for j := 0; j < columns && i+j < len(checks); j++ {
			row = append(row, container.New(layout.NewCustomPaddedLayout(0, 0, 0, spaceS), compactCheck(checks[i+j])))
		}
		rows.Add(checkColumns(columns, row...))
	}
	return rows
}
