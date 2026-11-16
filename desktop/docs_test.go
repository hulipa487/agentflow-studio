package main

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	studiodocs "agentflow-studio-docs"

	"agentflow-studio/internal/docs"
)

func TestEmbeddedDocs(t *testing.T) {
	lib, err := docs.New(studiodocs.FS, "")
	if err != nil {
		t.Fatal(err)
	}
	pages := lib.Pages()
	if len(pages) < 6 {
		t.Fatalf("docs pages = %d, want at least 6", len(pages))
	}
	for _, p := range pages {
		body, ok := lib.Read(p.Slug)
		if !ok || strings.TrimSpace(body) == "" {
			t.Errorf("%s: empty or unreadable", p.Slug)
		}
	}
	cfg, ok := lib.Read("configuration")
	if !ok {
		t.Fatal("configuration page missing")
	}
	if !strings.Contains(cfg, "agentflow.yaml") {
		t.Error("configuration page does not mention agentflow.yaml")
	}
	if strings.Contains(cfg, "internal/webui/docs") || strings.Contains(cfg, "served at `/docs/`") {
		t.Error("configuration page still points at the removed embedded docs site")
	}
	if _, err := lib.Asset("openapi/users.yaml"); err != nil {
		t.Errorf("OpenAPI asset: %v", err)
	}
	if _, err := lib.Asset("luau/af.d.luau"); err != nil {
		t.Errorf("Luau definitions asset: %v", err)
	}
}

func TestDocsSearchAndLinks(t *testing.T) {
	lib, err := docs.New(studiodocs.FS, "")
	if err != nil {
		t.Fatal(err)
	}
	results := lib.Search("named pipe", 10)
	if len(results) == 0 {
		t.Fatal("search for 'named pipe' returned nothing")
	}
	if _, ok := lib.SlugFor("configuration.md#local-transport"); !ok {
		t.Fatal("configuration.md link did not resolve")
	}
	if _, ok := lib.SlugFor("https://example.com"); ok {
		t.Fatal("external URL resolved as a page")
	}
}

// Every embedded Markdown file is a page, and every relative .md link in a page
// opens one: a file left out of the manifest is unreachable and every link to
// it is shown as plain text.
func TestEveryDocIsReachable(t *testing.T) {
	lib, err := docs.New(studiodocs.FS, "")
	if err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(studiodocs.FS, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if _, ok := lib.SlugFor(f); !ok {
			t.Errorf("%s is embedded but not in the docs manifest", f)
		}
	}
	link := regexp.MustCompile(`\]\(([^)\s]+)\)`)
	for _, p := range lib.Pages() {
		body, _ := lib.Read(p.Slug)
		for _, m := range link.FindAllStringSubmatch(body, -1) {
			href := m[1]
			if strings.Contains(href, "://") || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "mailto:") {
				continue
			}
			base := href
			if i := strings.IndexAny(base, "?#"); i >= 0 {
				base = base[:i]
			}
			if !strings.HasSuffix(base, ".md") {
				continue
			}
			if _, ok := lib.SlugFor(href); !ok {
				t.Errorf("%s links to %s, which is not a page", p.File, href)
			}
		}
	}
}
