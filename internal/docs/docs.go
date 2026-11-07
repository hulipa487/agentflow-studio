// Package docs loads the Markdown documentation embedded in AgentFlow Studio.
// The content lives at the repository root (docs/) because it is edited as
// ordinary files; main embeds it and hands the FS to this package so the viewer
// works offline and over a local admin socket.
package docs

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"unicode/utf8"
)

// Page is one rendered document.
type Page struct {
	Slug    string
	File    string
	Title   string
	Section string
	Order   int
}

// Result is one search hit.
type Result struct {
	Page    Page
	Snippet string
}

// Library is the embedded documentation set.
type Library struct {
	fs     fs.FS
	root   string
	pages  []Page
	bySlug map[string]Page
}

var manifest = []Page{
	{Slug: "getting-started", File: "getting-started.md", Title: "Overview", Section: "Start here", Order: 1},
	{Slug: "architecture", File: "architecture.md", Title: "Architecture & deployment", Section: "Start here", Order: 2},
	{Slug: "multi-region", File: "multi-region.md", Title: "Multi-region", Section: "Start here", Order: 3},
	{Slug: "loops", File: "loops.md", Title: "Luau loops", Section: "Build", Order: 4},
	{Slug: "extending", File: "extending.md", Title: "Extending", Section: "Build", Order: 5},
	{Slug: "configuration", File: "configuration.md", Title: "Configuration", Section: "Reference", Order: 6},
	{Slug: "loops-reference", File: "loops-reference.md", Title: "Loop API reference", Section: "Reference", Order: 7},
}

// New validates the manifest against fsys. root is the directory that holds
// the Markdown pages (normally "docs").
func New(fsys fs.FS, root string) (*Library, error) {
	l := &Library{fs: fsys, root: strings.Trim(root, "/"), bySlug: map[string]Page{}}
	for _, p := range manifest {
		body, err := fs.ReadFile(fsys, l.path(p.File))
		if err != nil {
			return nil, fmt.Errorf("docs: %s: %w", p.File, err)
		}
		if len(body) == 0 {
			return nil, fmt.Errorf("docs: %s is empty", p.File)
		}
		l.pages = append(l.pages, p)
		l.bySlug[p.Slug] = p
	}
	if _, err := fs.ReadFile(fsys, l.path("openapi/users.yaml")); err != nil {
		return nil, fmt.Errorf("docs: openapi/users.yaml: %w", err)
	}
	return l, nil
}

func (l *Library) path(name string) string {
	if l.root == "" {
		return name
	}
	return l.root + "/" + name
}

// Pages returns the manifest in reading order.
func (l *Library) Pages() []Page { return append([]Page(nil), l.pages...) }

// Page finds one page by slug.
func (l *Library) Page(slug string) (Page, bool) {
	p, ok := l.bySlug[slug]
	return p, ok
}

// Read returns a page's Markdown.
func (l *Library) Read(slug string) (string, bool) {
	p, ok := l.bySlug[slug]
	if !ok {
		return "", false
	}
	b, err := fs.ReadFile(l.fs, l.path(p.File))
	if err != nil {
		return "", false
	}
	return string(b), true
}

// Asset reads a bundled non-page file (the OpenAPI contract or the Luau type
// definitions). The name is relative to the docs root.
func (l *Library) Asset(name string) ([]byte, error) {
	return fs.ReadFile(l.fs, l.path(strings.TrimLeft(name, "/")))
}

// SlugFor maps a Markdown link target to a page slug. Only links within the
// embedded set resolve; external URLs and bare anchors do not.
func (l *Library) SlugFor(href string) (string, bool) {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.Contains(href, "://") || strings.HasPrefix(href, "mailto:") {
		return "", false
	}
	if i := strings.IndexAny(href, "?#"); i >= 0 {
		href = href[:i]
	}
	href = strings.TrimPrefix(href, "./")
	base := href
	if i := strings.LastIndexByte(base, '/'); i >= 0 {
		base = base[i+1:]
	}
	if !strings.HasSuffix(base, ".md") {
		return "", false
	}
	slug := strings.TrimSuffix(base, ".md")
	_, ok := l.bySlug[slug]
	return slug, ok
}

// Search finds pages containing query, case-insensitively, and returns a line
// of context for each. An empty query returns no results.
func (l *Library) Search(query string, limit int) []Result {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	if limit <= 0 {
		limit = 20
	}
	var out []Result
	for _, p := range l.pages {
		body, ok := l.Read(p.Slug)
		if !ok {
			continue
		}
		lower := strings.ToLower(body)
		idx := strings.Index(lower, q)
		if idx < 0 && !strings.Contains(strings.ToLower(p.Title), q) {
			continue
		}
		snippet := ""
		if idx >= 0 {
			start := strings.LastIndexByte(body[:idx], '\n') + 1
			end := strings.IndexByte(body[idx:], '\n')
			if end < 0 {
				end = len(body)
			} else {
				end += idx
			}
			snippet = strings.TrimSpace(body[start:end])
			if utf8.RuneCountInString(snippet) > 160 {
				r := []rune(snippet)
				snippet = string(r[:157]) + "…"
			}
		}
		out = append(out, Result{Page: p, Snippet: snippet})
		if len(out) >= limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Page.Order < out[j].Page.Order })
	return out
}
