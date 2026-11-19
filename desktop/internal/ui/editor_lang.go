package ui

import (
	"image/color"
	"regexp"
	"strings"
	"unicode"
)

// hlSpan is a run of text in one colour. kind marks comments ('c') and
// strings ('s'), which bracket matching skips.
type hlSpan struct {
	text   string
	color  color.Color
	kind   byte
	bold   bool
	italic bool
}

// langSpec is everything the editor knows about a language.
type langSpec struct {
	name         string
	unit         int // spaces per indent level
	lineComment  string
	blockComment [2]string
	pairs        map[rune]rune
	// highlight colours one line given the state the previous line ended in
	// (long strings, block scalars, code fences) and returns the next state.
	highlight func(line string, state int) ([]hlSpan, int)
	// opens reports a line after which the next one is indented a level.
	opens func(trimmed string) bool
	// closes reports a line that is a block closer, re-indented as it is
	// typed; nil when the language has none.
	closes func(trimmed string) bool
	// fold returns the last row a fold starting at row hides; nil folds by
	// indentation.
	fold func(lines []string, row int) (int, bool)
}

var (
	darkKeyword  = color.NRGBA{0xC5, 0x86, 0xC0, 0xFF}
	darkString   = color.NRGBA{0xCE, 0x91, 0x78, 0xFF}
	darkNumber   = color.NRGBA{0xB5, 0xCE, 0xA8, 0xFF}
	darkComment  = color.NRGBA{0x6A, 0x99, 0x55, 0xFF}
	darkFunction = color.NRGBA{0xDC, 0xDC, 0xAA, 0xFF}
	darkBuiltin  = color.NRGBA{0x4F, 0xC1, 0xFF, 0xFF}
	darkProperty = color.NRGBA{0x9C, 0xDC, 0xFE, 0xFF}

	lightKeyword  = color.NRGBA{0xAF, 0x00, 0xDB, 0xFF}
	lightString   = color.NRGBA{0xA3, 0x15, 0x15, 0xFF}
	lightNumber   = color.NRGBA{0x09, 0x86, 0x58, 0xFF}
	lightComment  = color.NRGBA{0x00, 0x80, 0x00, 0xFF}
	lightFunction = color.NRGBA{0x79, 0x5E, 0x26, 0xFF}
	lightBuiltin  = color.NRGBA{0x00, 0x70, 0xC1, 0xFF}
	lightProperty = color.NRGBA{0x00, 0x10, 0x80, 0xFF}
)

func synColor(dark, light color.NRGBA) color.Color {
	if activeTheme == themeLight {
		return light
	}
	return dark
}

func cKeyword() color.Color  { return synColor(darkKeyword, lightKeyword) }
func cString() color.Color   { return synColor(darkString, lightString) }
func cNumber() color.Color   { return synColor(darkNumber, lightNumber) }
func cComment() color.Color  { return synColor(darkComment, lightComment) }
func cFunction() color.Color { return synColor(darkFunction, lightFunction) }
func cBuiltin() color.Color  { return synColor(darkBuiltin, lightBuiltin) }
func cProperty() color.Color { return synColor(darkProperty, lightProperty) }

var languages = map[string]*langSpec{}

func init() {
	brackets := map[rune]rune{'(': ')', '[': ']', '{': '}'}
	withQuotes := func(q ...rune) map[rune]rune {
		m := map[rune]rune{}
		for k, v := range brackets {
			m[k] = v
		}
		for _, r := range q {
			m[r] = r
		}
		return m
	}
	languages["lua"] = &langSpec{
		name: "lua", unit: 2, lineComment: "--", blockComment: [2]string{"--[[", "]]"},
		pairs: withQuotes('"', '\''), highlight: highlightLuaState,
		opens: luaOpens, closes: luaCloses,
	}
	languages["yaml"] = &langSpec{
		name: "yaml", unit: 2, lineComment: "#", pairs: withQuotes('"', '\''),
		highlight: highlightYAML,
		opens: func(t string) bool {
			return strings.HasSuffix(t, ":") || strings.HasSuffix(t, "|") || strings.HasSuffix(t, ">") ||
				strings.HasSuffix(t, "|-") || strings.HasSuffix(t, ">-")
		},
	}
	languages["json"] = &langSpec{
		name: "json", unit: 2, pairs: map[rune]rune{'[': ']', '{': '}', '"': '"'},
		highlight: highlightJSON,
		opens:     func(t string) bool { return strings.HasSuffix(t, "{") || strings.HasSuffix(t, "[") },
		closes:    func(t string) bool { return t == "}" || t == "]" || t == "}," || t == "]," },
	}
	languages["markdown"] = &langSpec{
		name: "markdown", unit: 2, blockComment: [2]string{"<!--", "-->"},
		pairs: brackets, highlight: highlightMarkdown,
		opens: func(string) bool { return false }, fold: markdownFold,
	}
	languages["text"] = &langSpec{
		name: "text", unit: 2, pairs: brackets,
		highlight: func(line string, state int) ([]hlSpan, int) {
			return []hlSpan{{text: line, color: colorText}}, 0
		},
		opens: func(string) bool { return false },
	}
}

func languageFor(name string) *langSpec {
	if l, ok := languages[strings.ToLower(name)]; ok {
		return l
	}
	return languages["text"]
}

// ---- highlighter state cache ---------------------------------------------

// stateAt is the highlighter state at the start of row.
func (e *codeEditor) stateAt(row int) int {
	if len(e.hlStates) != len(e.lines) {
		next := make([]int, len(e.lines))
		copy(next, e.hlStates)
		e.hlStates = next
		if e.hlValid > len(e.lines) {
			e.hlValid = len(e.lines)
		}
	}
	if e.hlValid == 0 {
		e.hlStates[0] = 0
		e.hlValid = 1
	}
	for e.hlValid <= row && e.hlValid < len(e.lines) {
		r := e.hlValid
		_, st := e.lang.highlight(e.lines[r-1], e.hlStates[r-1])
		e.hlStates[r] = st
		e.hlValid++
	}
	return e.hlStates[row]
}

func (e *codeEditor) spansAt(row int) []hlSpan {
	spans, _ := e.lang.highlight(e.lines[row], e.stateAt(row))
	return spans
}

// inStringOrComment reports whether row:col is inside a string or comment.
func (e *codeEditor) inStringOrComment(row, col int) bool {
	at := 0
	for _, s := range e.spansAt(row) {
		n := runeLen(s.text)
		if col < at+n {
			return s.kind == 'c' || s.kind == 's'
		}
		at += n
	}
	return false
}

// spanBuilder accumulates spans for one line.
type spanBuilder struct{ out []hlSpan }

func (b *spanBuilder) add(text string, c color.Color, kind byte) {
	if text == "" {
		return
	}
	if n := len(b.out); n > 0 && b.out[n-1].color == c && b.out[n-1].kind == kind && !b.out[n-1].bold && !b.out[n-1].italic {
		b.out[n-1].text += text
		return
	}
	b.out = append(b.out, hlSpan{text: text, color: c, kind: kind})
}

func (b *spanBuilder) styled(text string, c color.Color, bold, italic bool) {
	if text != "" {
		b.out = append(b.out, hlSpan{text: text, color: c, bold: bold, italic: italic})
	}
}

// ---- Lua --------------------------------------------------------------------

// highlightLua colours one line from a clean state.
func highlightLua(line string) []hlSpan {
	spans, _ := highlightLuaState(line, 0)
	return spans
}

// longOpen matches a long bracket opener at rs[i:], returning its level.
func longOpen(rs []rune, i int) (int, int, bool) {
	if i >= len(rs) || rs[i] != '[' {
		return 0, 0, false
	}
	j := i + 1
	for j < len(rs) && rs[j] == '=' {
		j++
	}
	if j < len(rs) && rs[j] == '[' {
		return j - i - 1, j + 1, true
	}
	return 0, 0, false
}

// longClose finds the closer for a long bracket of level from rs[i:].
func longClose(rs []rune, i, level int) (int, bool) {
	closer := "]" + strings.Repeat("=", level) + "]"
	idx := strings.Index(string(rs[i:]), closer)
	if idx < 0 {
		return 0, false
	}
	return i + runeLen(string(rs[i:])[:idx]) + level + 2, true
}

// highlightLuaState colours a Luau line. State 0 is code; inside a long
// comment or string it is 1 + 2*level (+1 for a string), so the lines after
// --[[ stay comment-coloured until ]].
func highlightLuaState(line string, state int) ([]hlSpan, int) {
	rs := []rune(line)
	var b spanBuilder
	i := 0
	if state > 0 {
		level := (state - 1) / 2
		kind, c := byte('c'), cComment()
		if (state-1)%2 == 1 {
			kind, c = 's', cString()
		}
		end, ok := longClose(rs, 0, level)
		if !ok {
			b.add(line, c, kind)
			return b.out, state
		}
		b.add(string(rs[:end]), c, kind)
		i = end
	}
	punct := colorMuted
	for i < len(rs) {
		r := rs[i]
		if r == '-' && i+1 < len(rs) && rs[i+1] == '-' {
			if level, after, ok := longOpen(rs, i+2); ok {
				end, closed := longClose(rs, after, level)
				if !closed {
					b.add(string(rs[i:]), cComment(), 'c')
					return b.out, 1 + 2*level
				}
				b.add(string(rs[i:end]), cComment(), 'c')
				i = end
				continue
			}
			b.add(string(rs[i:]), cComment(), 'c')
			return b.out, 0
		}
		if level, after, ok := longOpen(rs, i); ok {
			end, closed := longClose(rs, after, level)
			if !closed {
				b.add(string(rs[i:]), cString(), 's')
				return b.out, 2 + 2*level
			}
			b.add(string(rs[i:end]), cString(), 's')
			i = end
			continue
		}
		if r == '"' || r == '\'' || r == '`' {
			j := i + 1
			for j < len(rs) {
				if rs[j] == '\\' {
					j += 2
					continue
				}
				if rs[j] == r {
					j++
					break
				}
				j++
			}
			if j > len(rs) {
				j = len(rs)
			}
			b.add(string(rs[i:j]), cString(), 's')
			i = j
			continue
		}
		if unicode.IsDigit(r) {
			j := i + 1
			for j < len(rs) && (unicode.IsDigit(rs[j]) || rs[j] == '.' || rs[j] == 'x' || rs[j] == 'X' || rs[j] == '_' ||
				(rs[j] >= 'a' && rs[j] <= 'f') || (rs[j] >= 'A' && rs[j] <= 'F')) {
				j++
			}
			b.add(string(rs[i:j]), cNumber(), 0)
			i = j
			continue
		}
		if isIdentRune(r) {
			j := i + 1
			for j < len(rs) && isIdentRune(rs[j]) {
				j++
			}
			word := string(rs[i:j])
			k := j
			for k < len(rs) && rs[k] == ' ' {
				k++
			}
			call := k < len(rs) && (rs[k] == '(' || rs[k] == '"' || rs[k] == '{')
			member := i > 0 && (rs[i-1] == '.' || rs[i-1] == ':')
			var c color.Color = colorText
			switch {
			case luaKeyword(word):
				c = cKeyword()
			case call:
				c = cFunction()
			case member:
				c = cProperty()
			case preludeRoot(word):
				c = cBuiltin()
			}
			b.add(word, c, 0)
			i = j
			continue
		}
		if unicode.IsSpace(r) {
			b.add(string(r), colorText, 0)
		} else {
			b.add(string(r), punct, 0)
		}
		i++
	}
	return b.out, 0
}

func luaKeyword(s string) bool {
	switch s {
	case "and", "break", "do", "else", "elseif", "end", "false", "for", "function",
		"if", "in", "local", "nil", "not", "or", "repeat", "return", "then", "true",
		"until", "while", "continue", "type", "export":
		return true
	}
	return false
}

func preludeRoot(s string) bool {
	switch s {
	case "session", "llm", "tools", "store", "shell", "http", "mail", "agent", "scheduler",
		"router", "runtime", "credential", "memory", "log", "json", "os", "files", "media",
		"string", "table", "math", "self", "require", "print", "pairs", "ipairs", "tostring", "tonumber":
		return true
	}
	return false
}

// luaOpens reports a line after which Luau indents: a block opener left open.
func luaOpens(t string) bool {
	t = stripLuaComment(t)
	for _, suffix := range []string{"then", "do", "else", "repeat", "{", "(", "["} {
		if strings.HasSuffix(t, suffix) && (len(suffix) == 1 || len(t) == len(suffix) || !isIdentRune(rune(t[len(t)-len(suffix)-1]))) {
			return true
		}
	}
	if strings.HasSuffix(t, "function") {
		return true
	}
	// function f(a, b) / local function f() / x = function() with no end.
	if i := strings.Index(t, "function"); i >= 0 && strings.HasSuffix(t, ")") && !strings.HasSuffix(t, "end") {
		return true
	}
	return false
}

func luaCloses(t string) bool {
	switch t {
	case "end", "else", "elseif", "until", "}", ")", "]", "end)", "},", "end,":
		return true
	}
	return false
}

func stripLuaComment(t string) string {
	if i := strings.Index(t, "--"); i >= 0 {
		return strings.TrimSpace(t[:i])
	}
	return t
}

// ---- YAML -------------------------------------------------------------------

var yamlKey = regexp.MustCompile(`^(\s*)(- +)?("[^"]*"|'[^']*'|[^\s#'"\-][^:#]*?|-[^\s:#][^:#]*?)(\s*):(\s|$)`)

// highlightYAML colours a YAML line. A block scalar (key: | or >) puts the
// state at its key's indent + 1, and the more-indented lines after it are
// string-coloured.
func highlightYAML(line string, state int) ([]hlSpan, int) {
	var b spanBuilder
	if state > 0 {
		if strings.TrimSpace(line) == "" || leadingSpaces(line) > state-1 {
			b.add(line, cString(), 's')
			return b.out, state
		}
		state = 0
	}
	trim := strings.TrimSpace(line)
	if strings.HasPrefix(trim, "#") {
		b.add(line, cComment(), 'c')
		return b.out, 0
	}
	if trim == "---" || trim == "..." {
		b.add(line, cKeyword(), 0)
		return b.out, 0
	}
	rest := line
	next := 0
	if m := yamlKey.FindStringSubmatchIndex(line); m != nil {
		b.add(line[:m[3]], colorText, 0)
		if m[4] >= 0 {
			b.add(line[m[4]:m[5]], colorMuted, 0)
		}
		b.add(line[m[6]:m[7]], cProperty(), 0)
		b.add(line[m[7]:m[9]+1], colorMuted, 0)
		rest = line[m[9]+1:]
		ind := leadingSpaces(line)
		if m[4] >= 0 {
			ind += m[5] - m[4]
		}
		v := strings.TrimSpace(stripYAMLComment(rest))
		if strings.HasPrefix(v, "|") || strings.HasPrefix(v, ">") {
			next = ind + 1
		}
	} else {
		ind := leadingSpaces(line)
		b.add(line[:ind], colorText, 0)
		rest = line[ind:]
		if strings.HasPrefix(rest, "- ") || rest == "-" {
			b.add("-", colorMuted, 0)
			rest = rest[1:]
		}
	}
	yamlValue(&b, rest)
	return b.out, next
}

func stripYAMLComment(s string) string {
	inS, inD := false, false
	for i, r := range s {
		switch {
		case r == '\'' && !inD:
			inS = !inS
		case r == '"' && !inS:
			inD = !inD
		case r == '#' && !inS && !inD && (i == 0 || s[i-1] == ' '):
			return s[:i]
		}
	}
	return s
}

func yamlValue(b *spanBuilder, s string) {
	body := stripYAMLComment(s)
	comment := s[len(body):]
	v := strings.TrimSpace(body)
	lead := body[:len(body)-len(strings.TrimLeft(body, " "))]
	trail := body[len(lead)+len(v):]
	b.add(lead, colorText, 0)
	switch {
	case v == "":
	case strings.HasPrefix(v, "\"") || strings.HasPrefix(v, "'"):
		b.add(v, cString(), 's')
	case v == "true" || v == "false" || v == "null" || v == "~" || v == "yes" || v == "no":
		b.add(v, cKeyword(), 0)
	case isYAMLNumber(v):
		b.add(v, cNumber(), 0)
	case strings.HasPrefix(v, "&") || strings.HasPrefix(v, "*") || strings.HasPrefix(v, "!"):
		b.add(v, cBuiltin(), 0)
	case strings.HasPrefix(v, "|") || strings.HasPrefix(v, ">"):
		b.add(v, cKeyword(), 0)
	case strings.HasPrefix(v, "[") || strings.HasPrefix(v, "{"):
		for _, r := range v {
			if strings.ContainsRune("[]{},:", r) {
				b.add(string(r), colorMuted, 0)
			} else {
				b.add(string(r), cString(), 0)
			}
		}
	default:
		b.add(v, cString(), 0)
	}
	b.add(trail, colorText, 0)
	b.add(comment, cComment(), 'c')
}

var yamlNumber = regexp.MustCompile(`^[-+]?(\d[\d_]*(\.\d*)?([eE][-+]?\d+)?|0x[0-9a-fA-F]+|\.inf|\.nan)$`)

func isYAMLNumber(s string) bool { return yamlNumber.MatchString(s) }

// ---- JSON -------------------------------------------------------------------

func highlightJSON(line string, _ int) ([]hlSpan, int) {
	rs := []rune(line)
	var b spanBuilder
	for i := 0; i < len(rs); {
		r := rs[i]
		switch {
		case r == '"':
			j := i + 1
			for j < len(rs) {
				if rs[j] == '\\' {
					j += 2
					continue
				}
				if rs[j] == '"' {
					j++
					break
				}
				j++
			}
			if j > len(rs) {
				j = len(rs)
			}
			k := j
			for k < len(rs) && rs[k] == ' ' {
				k++
			}
			c := cString()
			if k < len(rs) && rs[k] == ':' {
				c = cProperty()
			}
			b.add(string(rs[i:j]), c, 's')
			i = j
		case r == '-' || unicode.IsDigit(r):
			j := i + 1
			for j < len(rs) && (unicode.IsDigit(rs[j]) || strings.ContainsRune(".eE+-", rs[j])) {
				j++
			}
			b.add(string(rs[i:j]), cNumber(), 0)
			i = j
		case unicode.IsLetter(r):
			j := i + 1
			for j < len(rs) && unicode.IsLetter(rs[j]) {
				j++
			}
			b.add(string(rs[i:j]), cKeyword(), 0)
			i = j
		case unicode.IsSpace(r):
			b.add(string(r), colorText, 0)
			i++
		default:
			b.add(string(r), colorMuted, 0)
			i++
		}
	}
	return b.out, 0
}

// ---- Markdown -----------------------------------------------------------------

var (
	mdHeading = regexp.MustCompile(`^#{1,6}\s`)
	mdInline  = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|__[^_]+__|\\*[^*\\s][^*]*\\*|_[^_\\s][^_]*_|\\[[^\\]]*\\]\\([^)]*\\)|<!--.*?-->")
)

// highlightMarkdown colours prose lightly: headings, lists, quotes, inline
// code, emphasis and links. State 1 is inside a fenced code block.
func highlightMarkdown(line string, state int) ([]hlSpan, int) {
	var b spanBuilder
	trim := strings.TrimSpace(line)
	fence := strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~")
	if state == 1 {
		if fence {
			b.add(line, colorMuted, 0)
			return b.out, 0
		}
		b.add(line, cBuiltin(), 's')
		return b.out, 1
	}
	if fence {
		b.add(line, colorMuted, 0)
		return b.out, 1
	}
	switch {
	case mdHeading.MatchString(trim):
		b.styled(line, cKeyword(), true, false)
		return b.out, 0
	case strings.HasPrefix(trim, ">"):
		b.styled(line, cComment(), false, true)
		return b.out, 0
	case trim == "---" || trim == "***" || trim == "___":
		b.add(line, colorMuted, 0)
		return b.out, 0
	}
	rest := line
	if m := listMarker.FindStringIndex(line); m != nil {
		b.add(line[:m[1]], cKeyword(), 0)
		rest = line[m[1]:]
	}
	at := 0
	for _, m := range mdInline.FindAllStringIndex(rest, -1) {
		b.add(rest[at:m[0]], colorText, 0)
		tok := rest[m[0]:m[1]]
		switch {
		case strings.HasPrefix(tok, "`"):
			b.add(tok, cBuiltin(), 's')
		case strings.HasPrefix(tok, "**") || strings.HasPrefix(tok, "__"):
			b.styled(tok, colorText, true, false)
		case strings.HasPrefix(tok, "*") || strings.HasPrefix(tok, "_"):
			b.styled(tok, colorText, false, true)
		case strings.HasPrefix(tok, "<!--"):
			b.add(tok, cComment(), 'c')
		default:
			close := strings.Index(tok, "](")
			b.add(tok[:close+1], cProperty(), 0)
			b.add(tok[close+1:], cString(), 0)
		}
		at = m[1]
	}
	b.add(rest[at:], colorText, 0)
	return b.out, 0
}

// markdownFold folds a heading down to the next heading of its level or
// above, leaving trailing blank lines out.
func markdownFold(lines []string, row int) (int, bool) {
	t := strings.TrimSpace(lines[row])
	if !mdHeading.MatchString(t) {
		return 0, false
	}
	level := len(t) - len(strings.TrimLeft(t, "#"))
	end := row
	inFence := false
	for r := row + 1; r < len(lines); r++ {
		lt := strings.TrimSpace(lines[r])
		if strings.HasPrefix(lt, "```") || strings.HasPrefix(lt, "~~~") {
			inFence = !inFence
		}
		if !inFence && mdHeading.MatchString(lt) && len(lt)-len(strings.TrimLeft(lt, "#")) <= level {
			break
		}
		if lt != "" {
			end = r
		}
	}
	return end, end > row
}
