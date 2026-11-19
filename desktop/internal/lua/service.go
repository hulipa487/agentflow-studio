package lua

import (
	"fmt"
	"sort"
	"strings"

	"agentflow-studio/internal/i18n"
)

// CompletionItem is one suggestion for the editor.
type CompletionItem struct {
	Label         string
	Kind          string // function, field, variable, module, snippet
	Detail        string
	Documentation string
	Insert        string
}

// Hover is the documentation for the symbol at a position.
type Hover struct {
	Line, Col       int
	EndLine, EndCol int
	Markdown        string
}

func offsetFor(src []rune, line, col int) int {
	off := 0
	for l := 0; l < line; l++ {
		nl := indexRune(src[off:], '\n')
		if nl < 0 {
			return len(src)
		}
		off += nl + 1
	}
	if off+col > len(src) {
		return len(src)
	}
	return off + col
}

func indexRune(rs []rune, r rune) int {
	for i, v := range rs {
		if v == r {
			return i
		}
	}
	return -1
}

// Complete returns suggestions at a line/column. The prefix is taken from the
// text before the cursor; after a dot or colon it lists the fields of the
// prelude namespace named there.
func Complete(m *Model, line, col int) []CompletionItem {
	if m == nil {
		return nil
	}
	src := []rune(m.Source)
	off := offsetFor(src, line, col)
	start := off
	for start > 0 && isIdentRune(src[start-1]) {
		start--
	}
	prefix := string(src[start:off])

	path := ""
	if start > 0 && (src[start-1] == '.' || src[start-1] == ':') {
		dot := start - 1
		p := dot
		for p > 0 && isIdentRune(src[p-1]) {
			p--
		}
		path = string(src[p:dot])
		// Walk further back through a dotted chain (session.state.).
		for p > 0 && (src[p-1] == '.') {
			q := p - 1
			for q > 0 && isIdentRune(src[q-1]) {
				q--
			}
			if q == p-1 {
				break
			}
			path = string(src[q:p-1]) + "." + path
			p = q
		}
	}

	// A local variable shadows a prelude namespace of the same name.
	if path != "" {
		if localNamed(m, strings.Split(path, ".")[0], line, col) {
			return nil
		}
	}

	var items []CompletionItem
	seen := map[string]bool{}
	add := func(it CompletionItem) {
		if it.Label == "" || seen[it.Label] {
			return
		}
		if !strings.HasPrefix(strings.ToLower(it.Label), strings.ToLower(prefix)) {
			return
		}
		seen[it.Label] = true
		items = append(items, it)
	}

	if path != "" {
		for _, e := range Children(path) {
			add(completionFromEntry(e))
		}
		if len(items) == 0 {
			return nil
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
		return items
	}

	// Locals in scope win over globals.
	pos := Position{line, col}
	for _, l := range m.Locals {
		if l == nil || pos.Before(l.Decl) || !inScope(l, line, col) {
			continue
		}
		add(CompletionItem{Label: l.Name, Kind: "variable", Detail: l.Kind, Insert: l.Name})
	}
	for _, e := range Children("") {
		add(completionFromEntry(e))
	}
	for _, name := range StdlibCompletion() {
		kind := "function"
		if name == "string" || name == "table" || name == "math" || name == "utf8" ||
			name == "bit32" || name == "coroutine" || name == "buffer" || name == "vector" {
			kind = "module"
		}
		add(CompletionItem{Label: name, Kind: kind, Detail: i18n.T("lua.completion.stdlib"), Insert: name})
	}
	for name := range m.Globals {
		add(CompletionItem{Label: name, Kind: "function", Detail: i18n.T("lua.completion.chunkFunction"), Insert: name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func completionFromEntry(e *APIEntry) CompletionItem {
	label := e.Path
	if i := strings.LastIndex(label, "."); i >= 0 {
		label = label[i+1:]
	}
	kind := "field"
	switch e.Kind {
	case SymbolFunction:
		kind = "function"
	case SymbolTable:
		kind = "module"
	case SymbolVariable:
		kind = "variable"
	}
	doc := e.Summary
	if e.Capability != "" {
		doc += "\n\n" + i18n.T("lua.doc.capability", "capability", e.Capability)
	}
	if e.Maintenance {
		doc += "\n\n" + i18n.T("lua.doc.maintenanceShort")
	}
	insert := e.Insert
	if insert == "" {
		insert = label
	}
	return CompletionItem{
		Label: label, Kind: kind, Detail: e.Signature,
		Documentation: doc, Insert: insert,
	}
}

func localNamed(m *Model, name string, line, col int) bool {
	for _, l := range m.Locals {
		if l != nil && l.Name == name && inScope(l, line, col) {
			return true
		}
	}
	return false
}

func inScope(l *Local, line, col int) bool {
	p := Position{line, col}
	if p.Before(l.Start) {
		return false
	}
	if l.End != (Position{}) && l.End.Before(p) {
		return false
	}
	return true
}

// HoverAt describes the symbol at a line/column.
func HoverAt(m *Model, line, col int) *Hover {
	if m == nil {
		return nil
	}
	tokens := m.Tokens
	idx, lastOnLine := -1, -1
	for i, t := range tokens {
		if t.Kind == TokenEOF {
			break
		}
		if t.Line == line && t.Col <= col && col < t.EndCol {
			idx = i
			break
		}
		if t.Line < line && t.EndLine == line && col < t.EndCol {
			idx = i
			break
		}
		if t.EndLine == line && t.EndCol <= col {
			lastOnLine = i
			continue
		}
		if t.Line > line {
			break
		}
	}
	if idx < 0 {
		idx = lastOnLine
	}
	if idx < 0 || tokens[idx].Kind != TokenName {
		return nil
	}

	start := idx
	for start >= 2 && (tokens[start-1].Is(".") || tokens[start-1].Is(":")) && (tokens[start-2].Kind == TokenName || tokens[start-2].Kind == TokenKeyword) {
		start -= 2
	}
	end := idx
	for end+2 < len(tokens) && (tokens[end+1].Is(".") || tokens[end+1].Is(":")) && tokens[end+2].Kind == TokenName {
		end += 2
	}
	parts := make([]string, 0, (end-start)/2+1)
	for i := start; i <= end; i += 2 {
		parts = append(parts, tokens[i].Lit)
	}
	path := strings.Join(parts, ".")
	root := parts[0]

	h := &Hover{
		Line: tokens[start].Line, Col: tokens[start].Col,
		EndLine: tokens[end].EndLine, EndCol: tokens[end].EndCol,
	}

	if localNamed(m, root, line, col) {
		for _, l := range m.Locals {
			if l != nil && l.Name == root && inScope(l, line, col) {
				kind := "lua.hover.localVariable"
				switch l.Kind {
				case "param":
					kind = "lua.hover.parameter"
				case "loop":
					kind = "lua.hover.loopVariable"
				case "self":
					kind = "lua.hover.methodReceiver"
				case "function":
					kind = "lua.hover.localFunction"
				}
				h.Markdown = fmt.Sprintf("**%s**\n\n%s", l.Name, i18n.T("lua.hover.declaredAt", "kind", i18n.T(kind), "line", l.Decl.Line+1))
				return h
			}
		}
	}
	if e := Lookup(path); e != nil {
		h.Markdown = entryMarkdown(e)
		return h
	}
	if e := Lookup(root); e != nil {
		h.Markdown = fmt.Sprintf("**%s**\n\n%s", strings.Join(parts, "."), e.Summary)
		return h
	}
	if m.Globals[root] {
		h.Markdown = fmt.Sprintf("**%s**\n\n%s", root, i18n.T("lua.hover.chunkFunction"))
		return h
	}
	if IsKnownGlobal(root) {
		h.Markdown = fmt.Sprintf("**%s**\n\n%s", root, i18n.T("lua.hover.stdlib"))
		return h
	}
	return nil
}

func entryMarkdown(e *APIEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s**\n\n%s", e.Signature, e.Summary)
	if e.Capability != "" {
		b.WriteString("\n\n" + i18n.T("lua.doc.capabilityMarkdown", "capability", e.Capability))
	}
	if e.Maintenance {
		b.WriteString("\n\n*" + i18n.T("lua.doc.maintenance") + "*")
	}
	if e.Path == "llm.stream" {
		b.WriteString("\n\nCall `next_delta()` directly; a generic `for` fails across the bridge.")
	}
	return b.String()
}
