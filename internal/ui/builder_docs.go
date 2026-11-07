package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type builderDocPage struct {
	slug  string
	label string
}

var builderDocPages = []builderDocPage{
	{slug: "loops-reference", label: "Loop API reference"},
	{slug: "loops", label: "Luau coding tutorial"},
}

// luaDocsPane is the builder's inline documentation pane. The guide selector
// stays pinned at the top of the card while the reference body scrolls below.
func (b *builderView) luaDocsPane() fyne.CanvasObject {
	body := vstack(spaceS)
	var scroll *container.Scroll
	build := func(slug string) {
		clearBox(body)
		if b.app.docs == nil {
			body.Add(muted("Documentation is not bundled in this build."))
			body.Refresh()
			return
		}
		if _, ok := b.app.docs.Page(slug); !ok {
			body.Add(muted("This guide is not bundled in this build."))
			body.Refresh()
			return
		}
		source, ok := b.app.docs.Read(slug)
		if !ok {
			body.Add(muted("This guide could not be read."))
			body.Refresh()
			return
		}
		for _, obj := range b.app.renderMarkdownEmbedded(slug, source) {
			body.Add(obj)
		}
		body.Refresh()
		if scroll != nil {
			scroll.ScrollToTop()
		}
	}

	labels := make([]string, len(builderDocPages))
	for i, p := range builderDocPages {
		labels[i] = p.label
	}
	picker := widget.NewSelect(labels, nil)
	picker.PlaceHolder = "Choose a guide"
	picker.SetSelectedIndex(0)
	picker.OnChanged = func(label string) {
		for _, p := range builderDocPages {
			if p.label == label {
				build(p.slug)
				return
			}
		}
	}

	titleRow := container.NewBorder(nil, nil,
		vCenter(insetLeading(titleText("Luau Reference", theme.SizeNameSubHeadingText))),
		vCenter(chip("F1 Full Docs", toneNeutral)),
		nil,
	)
	head := vstack(spaceS, titleRow, controlFrame(picker), widget.NewSeparator())
	scroll = container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(280, 0))
	build(builderDocPages[0].slug)
	return panel(container.NewBorder(head, nil, nil, nil, scroll))
}
