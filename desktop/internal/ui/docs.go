package ui

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"

	"agentflow-studio/internal/docs"
)

// SetDocs installs the documentation set: the shared docs module's file
// system, with the guides at its root.
func (a *App) SetDocs(fsys fs.FS) error {
	lib, err := docs.New(fsys, "")
	if err != nil {
		return err
	}
	a.docs = lib
	return nil
}

// openDocs replaces the old "open /docs in a browser" action. Documentation is
// now part of Studio, so it works with a socket-only admin transport and with
// no network at all.
func (a *App) openDocs() {
	if a.docs == nil {
		a.setStatus("documentation is not bundled in this build", toneDanger)
		return
	}
	pages := a.docs.Pages()
	if len(pages) == 0 {
		a.setStatus("no documentation pages found", toneDanger)
		return
	}
	a.showDocs(pages[0].Slug)
}

func (a *App) showDocs(slug string) {
	if a.docs == nil {
		return
	}
	if _, ok := a.docs.Page(slug); !ok {
		pages := a.docs.Pages()
		if len(pages) == 0 {
			return
		}
		slug = pages[0].Slug
	}
	a.enter("docs")
	a.setMain(a.docsView(slug))
}

func (a *App) docsView(slug string) fyne.CanvasObject {
	sidebarList := container.NewVBox()
	sidebarScroll := container.NewVScroll(sidebarList)
	sidebarScroll.SetMinSize(fyne.NewSize(260, 0))

	page, _ := a.docs.Page(slug)

	var renderSidebar func(query string)
	renderSidebar = func(query string) {
		clearBox(sidebarList)
		if strings.TrimSpace(query) != "" {
			results := a.docs.Search(query, 30)
			sidebarList.Add(caption(fmt.Sprintf("%d result(s)", len(results))))
			if len(results) == 0 {
				sidebarList.Add(centerNote("No matching pages."))
				return
			}
			for _, r := range results {
				slug := r.Page.Slug
				label := r.Page.Title
				if r.Snippet != "" {
					label += "\n" + r.Snippet
				}
				b := widget.NewButton(label, func() { a.showDocs(slug) })
				b.Alignment = widget.ButtonAlignLeading
				sidebarList.Add(b)
			}
			return
		}
		currentSection := ""
		for _, p := range a.docs.Pages() {
			if p.Section != currentSection {
				currentSection = p.Section
				sidebarList.Add(caption(p.Section))
			}
			slug := p.Slug
			b := widget.NewButton(p.Title, func() { a.showDocs(slug) })
			b.Alignment = widget.ButtonAlignLeading
			if p.Slug == page.Slug {
				b.Importance = widget.HighImportance
			} else {
				b.Importance = widget.LowImportance
			}
			sidebarList.Add(b)
		}
	}
	sbox := newSearchBox("Search documentation…", func(q string) {
		renderSidebar(q)
	})
	searchField := sbox.Root
	renderSidebar("")

	sidebarBody := container.NewBorder(
		vstack(spaceXS,
			insetLeading(titleText("Documentation", theme.SizeNameSubHeadingText)),
			searchField,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		sidebarScroll,
	)
	sidebar := container.New(layout.NewCustomPaddedLayout(pageTop, pageBottom, pageGutter, spaceS), panel(sidebarBody))

	body, ok := a.docs.Read(slug)
	var pageCol fyne.CanvasObject
	if !ok {
		pageCol = vstack(spaceM, muted("This page could not be read."))
	} else {
		head := vstack(spaceXS,
			hstack(spaceS, vCenter(chip(page.Section, toneAccent))),
			insetLeading(titleText(page.Title, theme.SizeNameHeadingText)),
		)
		blocks := a.renderMarkdown(slug, body)
		pageCol = vstack(spaceM, append([]fyne.CanvasObject{head, widget.NewSeparator()}, blocks...)...)
	}
	pageScroll := container.NewVScroll(container.New(
		layout.NewCustomPaddedLayout(pageTop, pageBottom, spaceS, pageGutter), panel(pageCol)))

	split := container.NewHSplit(sidebar, pageScroll)
	split.SetOffset(0.28)
	return split
}

// renderMarkdown converts the supported Markdown into native Fyne widgets.
// RichText gives us selectable text and hyperlinks; headings, code, lists,
// quotes and tables get their own block widgets.
func (a *App) renderMarkdown(slug, source string) []fyne.CanvasObject {
	return a.renderMarkdownMode(slug, source, false)
}

// renderMarkdownEmbedded renders a page for an in-app reference pane. Internal
// page links become plain text so a click cannot replace the screen the reader
// is working in; external links still open in the browser.
func (a *App) renderMarkdownEmbedded(slug, source string) []fyne.CanvasObject {
	return a.renderMarkdownMode(slug, source, true)
}

func (a *App) renderMarkdownMode(slug, source string, embedded bool) []fyne.CanvasObject {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	src := []byte(source)
	doc := md.Parser().Parse(text.NewReader(src))
	r := &mdRenderer{app: a, slug: slug, source: src, embedded: embedded}
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		r.appendBlock(n)
	}
	return r.objects
}

type mdRenderer struct {
	app      *App
	slug     string
	source   []byte
	embedded bool
	objects  []fyne.CanvasObject
}

func (r *mdRenderer) append(obj fyne.CanvasObject) {
	if obj != nil {
		r.objects = append(r.objects, obj)
	}
}

func (r *mdRenderer) appendBlock(n ast.Node) {
	switch n := n.(type) {
	case *ast.Heading:
		r.append(r.heading(n))
	case *ast.Paragraph:
		segs := r.inline(n, textStyle{})
		if len(segs) > 0 {
			rt := widget.NewRichText(segs...)
			rt.Wrapping = fyne.TextWrapWord
			r.append(rt)
		}
	case *ast.TextBlock:
		segs := r.inline(n, textStyle{})
		if len(segs) > 0 {
			rt := widget.NewRichText(segs...)
			rt.Wrapping = fyne.TextWrapWord
			r.append(rt)
		}
	case *ast.FencedCodeBlock:
		r.append(r.codeBlock(n, n.Language(r.source)))
	case *ast.CodeBlock:
		r.append(r.codeBlock(n, nil))
	case *ast.List:
		r.append(r.list(n))
	case *ast.Blockquote:
		r.append(r.blockquote(n))
	case *ast.ThematicBreak:
		r.append(widget.NewSeparator())
	case *extast.Table:
		r.append(r.table(n))
	default:
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			r.appendBlock(c)
		}
	}
}

func (r *mdRenderer) heading(n *ast.Heading) fyne.CanvasObject {
	size := theme.SizeNameSubHeadingText
	if n.Level <= 1 {
		size = theme.SizeNameHeadingText
	}
	text := segmentsText(r.inline(n, textStyle{size: size, bold: true}))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	// A Label rather than RichText: labels wrap at the column width, while a
	// RichText heading sizes itself to its shortest line and breaks mid-phrase.
	lbl := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	lbl.SizeName = size
	lbl.Wrapping = fyne.TextWrapWord
	return lbl
}

func (r *mdRenderer) codeBlock(n ast.Node, lang []byte) fyne.CanvasObject {
	var b strings.Builder
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(r.source))
	}
	code := strings.TrimRight(b.String(), "\n")
	label := widget.NewRichText(&widget.TextSegment{
		Text: code,
		Style: widget.RichTextStyle{
			TextStyle: fyne.TextStyle{Monospace: true},
			SizeName:  theme.SizeNameText,
		},
	})
	label.Wrapping = fyne.TextWrapWord

	header := container.NewHBox()
	if len(lang) > 0 {
		header.Add(caption(strings.ToUpper(string(lang))))
	}
	header.Add(layout.NewSpacer())
	copyBtn := widget.NewButtonWithIcon("Copy", theme.ContentCopyIcon(), func() {
		r.app.win.Clipboard().SetContent(code)
		r.app.setStatus("code copied", toneSuccess)
	})
	copyBtn.Importance = widget.LowImportance
	header.Add(copyBtn)

	body := container.NewVBox(header, label)
	bg := canvas.NewRectangle(colorSurface2)
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	return container.NewStack(bg, container.NewPadded(body))
}

func (r *mdRenderer) list(n *ast.List) fyne.CanvasObject {
	box := container.NewVBox()
	num := 1
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		bullet := "•"
		if n.IsOrdered() {
			bullet = fmt.Sprintf("%d.", num)
			num++
		}
		box.Add(r.listItem(li, bullet, 0))
	}
	return box
}

func (r *mdRenderer) listItem(li *ast.ListItem, bullet string, depth int) fyne.CanvasObject {
	box := container.NewVBox()
	var segs []widget.RichTextSegment
	prefix := strings.Repeat("    ", depth) + bullet + " "
	segs = append(segs, &widget.TextSegment{
		Text: prefix,
		Style: widget.RichTextStyle{
			TextStyle: fyne.TextStyle{Bold: bullet != "•"},
			SizeName:  theme.SizeNameText,
			Inline:    true,
		},
	})
	hadInline := false
	for c := li.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Paragraph, *ast.TextBlock:
			segs = append(segs, r.inline(c, textStyle{})...)
			hadInline = true
		case *ast.List:
			if hadInline {
				rt := widget.NewRichText(segs...)
				rt.Wrapping = fyne.TextWrapWord
				box.Add(rt)
				segs = nil
				hadInline = false
			}
			box.Add(r.nestedList(c, depth+1))
		default:
			box.Add(r.blockObject(c))
		}
	}
	if len(segs) > 0 {
		rt := widget.NewRichText(segs...)
		rt.Wrapping = fyne.TextWrapWord
		box.Add(rt)
	}
	return box
}

func (r *mdRenderer) nestedList(n *ast.List, depth int) fyne.CanvasObject {
	box := container.NewVBox()
	num := 1
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}
		bullet := "◦"
		if n.IsOrdered() {
			bullet = fmt.Sprintf("%d.", num)
			num++
		}
		box.Add(r.listItem(li, bullet, depth))
	}
	return box
}

func (r *mdRenderer) blockquote(n *ast.Blockquote) fyne.CanvasObject {
	inner := &mdRenderer{app: r.app, slug: r.slug, source: r.source, embedded: r.embedded}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		inner.appendBlock(c)
	}
	body := container.NewVBox(inner.objects...)
	bar := canvas.NewRectangle(theme.Color(theme.ColorNamePrimary))
	bar.SetMinSize(fyne.NewSize(3, 0))
	return container.NewBorder(nil, nil, bar, nil, container.NewPadded(body))
}

func (r *mdRenderer) table(n *extast.Table) fyne.CanvasObject {
	var header []fyne.CanvasObject
	var rows [][]fyne.CanvasObject
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *extast.TableHeader:
			for cell := c.FirstChild(); cell != nil; cell = cell.NextSibling() {
				header = append(header, r.tableCell(cell, true))
			}
		case *extast.TableRow:
			var cells []fyne.CanvasObject
			for cell := c.FirstChild(); cell != nil; cell = cell.NextSibling() {
				cells = append(cells, r.tableCell(cell, false))
			}
			rows = append(rows, cells)
		}
	}
	cols := len(header)
	if cols == 0 {
		return nil
	}
	tableRows := vstack(0)
	headerGrid := container.NewGridWithColumns(cols, header...)
	headerBg := canvas.NewRectangle(colorSurface)
	headerBg.CornerRadius = 6
	tableRows.Add(container.NewStack(headerBg, headerGrid))
	for _, row := range rows {
		tableRows.Add(widget.NewSeparator())
		rowCells := make([]fyne.CanvasObject, cols)
		for i := 0; i < cols; i++ {
			if i < len(row) {
				rowCells[i] = row[i]
			} else {
				rowCells[i] = widget.NewLabel("")
			}
		}
		tableRows.Add(container.NewGridWithColumns(cols, rowCells...))
	}
	bg := canvas.NewRectangle(colorSurface2)
	bg.CornerRadius = 8
	bg.StrokeColor = colorBorder
	bg.StrokeWidth = 1
	return container.NewStack(bg, container.NewPadded(tableRows))
}

func (r *mdRenderer) tableCell(n ast.Node, header bool) fyne.CanvasObject {
	segs := r.inline(n, textStyle{bold: header})
	if len(segs) == 0 {
		segs = []widget.RichTextSegment{&widget.TextSegment{Text: ""}}
	}
	rt := widget.NewRichText(segs...)
	rt.Wrapping = fyne.TextWrapWord
	return container.NewPadded(rt)
}

type textStyle struct {
	bold   bool
	italic bool
	size   fyne.ThemeSizeName
}

func (s textStyle) merge(bold, italic bool) textStyle {
	if bold {
		s.bold = true
	}
	if italic {
		s.italic = true
	}
	return s
}

func (r *mdRenderer) inline(n ast.Node, style textStyle) []widget.RichTextSegment {
	var out []widget.RichTextSegment
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch c := c.(type) {
		case *ast.Text:
			t := string(c.Segment.Value(r.source))
			if t != "" {
				out = append(out, r.text(t, style))
			}
			if c.SoftLineBreak() || c.HardLineBreak() {
				out = append(out, r.text("\n", style))
			}
		case *ast.String:
			if len(c.Value) > 0 {
				out = append(out, r.text(string(c.Value), style))
			}
		case *ast.CodeSpan:
			var b strings.Builder
			for t := c.FirstChild(); t != nil; t = t.NextSibling() {
				if s, ok := t.(*ast.Text); ok {
					b.Write(s.Segment.Value(r.source))
				} else if s, ok := t.(*ast.String); ok {
					b.Write(s.Value)
				}
			}
			out = append(out, &widget.TextSegment{
				Text: b.String(),
				Style: widget.RichTextStyle{
					TextStyle: fyne.TextStyle{Monospace: true},
					SizeName:  theme.SizeNameText,
					ColorName: theme.ColorNamePrimary,
					Inline:    true,
				},
			})
		case *ast.Emphasis:
			out = append(out, r.inline(c, style.merge(c.Level >= 2, c.Level == 1))...)
		case *ast.Link:
			label := r.inline(c, style)
			text := segmentsText(label)
			if text == "" {
				text = string(c.Destination)
			}
			out = append(out, r.link(text, string(c.Destination), style))
		case *ast.AutoLink:
			u := string(c.URL(r.source))
			out = append(out, r.link(u, u, style))
		case *ast.Image:
			alt := segmentsText(r.inline(c, style))
			if alt == "" {
				alt = "image"
			}
			out = append(out, r.text("["+alt+"]", style))
		case *ast.RawHTML:
			// Raw HTML is deliberately not interpreted; the offline content is
			// Markdown and an embedded renderer should not execute markup.
		default:
			out = append(out, r.inline(c, style)...)
		}
	}
	return out
}

func segmentsText(segs []widget.RichTextSegment) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.Textual())
	}
	return b.String()
}

func (r *mdRenderer) text(s string, style textStyle) *widget.TextSegment {
	size := style.size
	if size == "" {
		size = theme.SizeNameText
	}
	return &widget.TextSegment{
		Text: s,
		Style: widget.RichTextStyle{
			TextStyle: fyne.TextStyle{Bold: style.bold, Italic: style.italic},
			SizeName:  size,
			Inline:    true,
		},
	}
}

func (r *mdRenderer) link(label, href string, style textStyle) widget.RichTextSegment {
	u, _ := url.Parse(href)
	seg := &widget.HyperlinkSegment{Text: label, URL: u}
	seg.TextStyle = fyne.TextStyle{Bold: style.bold, Italic: style.italic}
	seg.SizeName = theme.SizeNameText
	if slug, ok := r.app.docs.SlugFor(href); ok {
		if r.embedded {
			return r.text(label, style)
		}
		seg.OnTapped = func() { r.app.showDocs(slug) }
		return seg
	}
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") || strings.HasPrefix(href, "mailto:") {
		seg.OnTapped = func() {
			if u != nil {
				_ = r.app.fyneApp.OpenURL(u)
			}
		}
		return seg
	}
	if strings.HasPrefix(href, "openapi/") || strings.HasPrefix(href, "luau/") {
		if r.embedded {
			return r.text(label, style)
		}
		asset := href
		seg.OnTapped = func() { r.app.openDocsAsset(asset) }
		return seg
	}
	// Unknown relative link: show the label without a dead hyperlink.
	return r.text(label, style)
}

// openDocsAsset writes a bundled non-page file to a temp path and opens it with
// the OS handler (the OpenAPI contract or the Luau type definitions).
func (a *App) openDocsAsset(name string) {
	if a.docs == nil {
		return
	}
	b, err := a.docs.Asset(name)
	if err != nil {
		a.setStatus("could not read "+name, toneDanger)
		return
	}
	path := filepath.Join(os.TempDir(), "agentflow-studio-"+filepath.Base(name))
	if err := os.WriteFile(path, b, 0o600); err != nil {
		a.setStatus("could not write "+name+": "+err.Error(), toneDanger)
		return
	}
	u := &url.URL{Scheme: "file", Path: filepath.ToSlash(path)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path // Windows drive paths need the URL root
	}
	if err := a.fyneApp.OpenURL(u); err != nil {
		a.setStatus("could not open "+name+": "+err.Error(), toneDanger)
		return
	}
	a.setStatus("opened "+filepath.Base(path), toneSuccess)
}

func (r *mdRenderer) blockObject(n ast.Node) fyne.CanvasObject {
	switch n := n.(type) {
	case *ast.Paragraph:
		segs := r.inline(n, textStyle{})
		if len(segs) == 0 {
			return nil
		}
		rt := widget.NewRichText(segs...)
		rt.Wrapping = fyne.TextWrapWord
		return rt
	case *ast.FencedCodeBlock:
		return r.codeBlock(n, n.Language(r.source))
	default:
		inner := &mdRenderer{app: r.app, slug: r.slug, source: r.source, embedded: r.embedded}
		inner.appendBlock(n)
		if len(inner.objects) == 1 {
			return inner.objects[0]
		}
		return container.NewVBox(inner.objects...)
	}
}
