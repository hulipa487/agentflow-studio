package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type builderDocPage struct {
	slug     string
	labelKey string
}

var builderDocPages = []builderDocPage{
	{slug: "loops-reference", labelKey: "builder.docLoopsReference"},
	{slug: "loops", labelKey: "builder.docLoopsTutorial"},
}

// luaDocsPane is the builder's inline documentation pane, shown in the dock
// (F1). The guide selector stays pinned at the top of the card while the
// reference body scrolls below; "Full docs" opens the guide in the viewer.
func (b *builderView) luaDocsPane() fyne.CanvasObject {
	body := vstack(spaceS)
	var scroll *container.Scroll
	current := builderDocPages[0].slug
	build := func(slug string) {
		current = slug
		clearBox(body)
		if b.app.docs == nil {
			body.Add(muted(T("builder.docsNotBundled")))
			body.Refresh()
			return
		}
		if _, ok := b.app.docs.Page(slug); !ok {
			body.Add(muted(T("builder.guideNotBundled")))
			body.Refresh()
			return
		}
		source, ok := b.app.docs.Read(slug)
		if !ok {
			body.Add(muted(T("builder.guideUnreadable")))
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
		labels[i] = T(p.labelKey)
	}
	picker := widget.NewSelect(labels, nil)
	picker.PlaceHolder = T("builder.chooseGuide")
	picker.SetSelectedIndex(0)
	picker.OnChanged = func(label string) {
		for i, p := range builderDocPages {
			if labels[i] == label {
				build(p.slug)
				return
			}
		}
	}

	titleRow := container.NewBorder(nil, nil,
		vCenter(insetLeading(titleText(T("builder.luauReference"), theme.SizeNameSubHeadingText))),
		vCenter(func() fyne.CanvasObject {
			full := actionButton(T("builder.fullDocs"), theme.FileTextIcon(), func() { b.leave(func() { b.app.showDocs(current) }) })
			full.Importance = widget.LowImportance
			return full
		}()),
		nil,
	)
	head := vstack(spaceS, titleRow, controlFrame(picker), widget.NewSeparator())
	scroll = container.NewVScroll(body)
	scroll.SetMinSize(fyne.NewSize(280, 0))
	build(builderDocPages[0].slug)
	return panel(container.NewBorder(head, nil, nil, nil, scroll))
}
