package ui

import (
	"os"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"agentflow-studio/internal/appconfig"
)

func TestRenderMarkdownBlocks(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("docs")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	if err := a.SetDocs(os.DirFS("../../../docs")); err != nil {
		t.Fatal(err)
	}

	src := "# Title\n\nA **bold** and `code` [link](configuration.md).\n\n" +
		"| A | B |\n| --- | --- |\n| 1 | 2 |\n\n" +
		"- one\n- two\n\n> quoted\n\n```lua\nfunction loop() end\n```\n"
	objects := a.renderMarkdown("getting-started", src)
	if len(objects) < 6 {
		t.Fatalf("rendered %d objects, want headings, paragraphs, table, list, quote and code", len(objects))
	}
	for i, obj := range objects {
		if obj == nil {
			t.Errorf("object %d is nil", i)
		}
	}
}

func TestDocsSearchSidebarBuilds(t *testing.T) {
	fyneApp := test.NewApp()
	defer fyneApp.Quit()
	win := fyneApp.NewWindow("docs")
	a := New(fyneApp, win, appconfig.Config{Theme: "dark"})
	if err := a.SetDocs(os.DirFS("../../../docs")); err != nil {
		t.Fatal(err)
	}
	a.content = container.NewStack()
	a.showDocs("getting-started")
	if a.view != "docs" {
		t.Fatalf("view = %q, want docs", a.view)
	}
	if a.content == nil || len(a.content.Objects) != 1 {
		t.Fatal("docs view was not installed")
	}
}
