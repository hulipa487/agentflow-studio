package main

import (
	"strings"
	"testing"

	"agentflow-studio/internal/docs"
)

func TestEmbeddedDocs(t *testing.T) {
	lib, err := docs.New(studioDocsFS, "docs")
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
	lib, err := docs.New(studioDocsFS, "docs")
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
