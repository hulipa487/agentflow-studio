package main

import (
	"io/fs"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	studiodocs "agentflow-studio-docs"
)

// The translated guides must match their English source in structure: the
// same headings (the apps give a translated heading the English heading's
// anchor by position), the same code blocks (only prose comments may be
// translated), the same link targets and the same table shapes. The web app
// has the same check (web/src/docs/translations.test.ts).

type guideShape struct {
	headings []int
	blocks   []codeBlock
	links    []string
	tables   [][]int
}

type codeBlock struct {
	info string
	code []string
}

var (
	fenceLine   = regexp.MustCompile("^(\\s*)(`{3,}|~{3,})(.*)$")
	headingLine = regexp.MustCompile(`^(#{1,6})\s+\S`)
	linkTarget  = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
)

func parseGuide(text string) guideShape {
	var g guideShape
	var (
		inFence bool
		fence   string
		block   codeBlock
		table   []int
	)
	flushTable := func() {
		if table != nil {
			g.tables = append(g.tables, table)
			table = nil
		}
	}
	for _, line := range strings.Split(text, "\n") {
		m := fenceLine.FindStringSubmatch(line)
		if inFence {
			if m != nil && m[2][0] == fence[0] && len(m[2]) >= len(fence) && strings.TrimSpace(m[3]) == "" {
				g.blocks = append(g.blocks, block)
				inFence = false
			} else {
				block.code = append(block.code, line)
			}
			continue
		}
		if m != nil {
			inFence, fence, block = true, m[2], codeBlock{info: strings.TrimSpace(m[3])}
			continue
		}
		if h := headingLine.FindStringSubmatch(line); h != nil {
			g.headings = append(g.headings, len(h[1]))
		}
		for _, l := range linkTarget.FindAllStringSubmatch(line, -1) {
			g.links = append(g.links, l[1])
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "|") {
			table = append(table, strings.Count(strings.Trim(strings.TrimSpace(line), "|"), "|")+1)
		} else {
			flushTable()
		}
	}
	flushTable()
	sort.Strings(g.links)
	return g
}

// stripComment drops a comment that starts at the line's beginning or after
// whitespace: #, -- or //. Directives stay: #!, #{, and -- af-…. A shell
// flag (--flag) reads as a Luau comment too; both sides of a comparison are
// stripped alike, so that only makes the check blind to flag text.
func stripComment(line string) string {
	for i := 0; i < len(line); i++ {
		if i > 0 && line[i-1] != ' ' && line[i-1] != '\t' {
			continue
		}
		rest := line[i:]
		switch {
		case strings.HasPrefix(rest, "#") && !strings.HasPrefix(rest, "#!") && !strings.HasPrefix(rest, "#{"):
		case strings.HasPrefix(rest, "--") && !strings.HasPrefix(strings.TrimLeft(rest[2:], " "), "af-"):
		case strings.HasPrefix(rest, "//"):
		default:
			continue
		}
		return line[:i]
	}
	return line
}

func codeLines(code []string) []string {
	var out []string
	for _, l := range code {
		if s := strings.TrimSpace(stripComment(l)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func TestGuidesHaveMatchingTranslations(t *testing.T) {
	files, err := fs.Glob(studiodocs.FS, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh-Hans", "zh-Hant"} {
		for _, file := range files {
			en, _ := fs.ReadFile(studiodocs.FS, file)
			tr, err := fs.ReadFile(studiodocs.FS, lang+"/"+file)
			if err != nil {
				t.Errorf("%s has no %s translation (%s/%s)", file, lang, lang, file)
				continue
			}
			a, b := parseGuide(string(en)), parseGuide(string(tr))
			name := lang + "/" + file
			if !reflect.DeepEqual(a.headings, b.headings) {
				t.Errorf("%s: headings differ from the English (%d vs %d); a translation keeps every heading, in order, at the same level", name, len(b.headings), len(a.headings))
			}
			if len(a.blocks) != len(b.blocks) {
				t.Errorf("%s: %d code blocks, the English has %d", name, len(b.blocks), len(a.blocks))
			} else {
				for i := range a.blocks {
					if a.blocks[i].info != b.blocks[i].info {
						t.Errorf("%s: code block %d is %q, the English is %q", name, i+1, b.blocks[i].info, a.blocks[i].info)
					} else if !reflect.DeepEqual(codeLines(a.blocks[i].code), codeLines(b.blocks[i].code)) {
						t.Errorf("%s: code block %d (%s) differs from the English beyond its comments", name, i+1, a.blocks[i].info)
					}
				}
			}
			if !reflect.DeepEqual(a.links, b.links) {
				t.Errorf("%s: link targets differ from the English (%d vs %d)", name, len(b.links), len(a.links))
			}
			if !reflect.DeepEqual(a.tables, b.tables) {
				t.Errorf("%s: tables differ from the English in shape", name)
			}
		}
		extra, _ := fs.Glob(studiodocs.FS, lang+"/*.md")
		for _, f := range extra {
			if _, err := fs.Stat(studiodocs.FS, strings.TrimPrefix(f, lang+"/")); err != nil {
				t.Errorf("%s has no English source", f)
			}
		}
	}
}

func TestStripComment(t *testing.T) {
	cases := map[string]string{
		"key: value  # a note":       "key: value  ",
		"# whole line":               "",
		"#!/bin/sh":                  "#!/bin/sh",
		"local x = 1 -- why":         "local x = 1 ",
		"-- af-prelude-version: 1":   "-- af-prelude-version: 1",
		"url: http://host/x":         "url: http://host/x",
		"a // b":                     "a ",
		"color: \"#fff\"":            "color: \"#fff\"",
		"run --flag value":           "run ",
		"${VAR:-default} # fallback": "${VAR:-default} ",
	}
	for in, want := range cases {
		if got := stripComment(in); got != want {
			t.Errorf("stripComment(%q) = %q, want %q", in, got, want)
		}
	}
}
