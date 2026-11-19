package i18n

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The message syntax both Studio apps share (the web twin is
// web/src/i18n/format.ts; keep the two in step).
//
//	{name}                                    a parameter
//	{n, plural, =0 {…} one {…} other {…}}     a count; # prints it
//	{kind, select, a {…} b {…} other {…}}     a choice
//
// A brace that does not open one of those forms is literal text, so JSON and
// YAML samples can appear in a message as they are.

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// ident returns the identifier s starts with, or "".
func ident(s string) string {
	if s == "" || !isIdentStart(s[0]) {
		return ""
	}
	i := 1
	for i < len(s) && (isIdentStart(s[i]) || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	return s[:i]
}

// closing returns the end of the brace group opening at open, or -1.
func closing(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

type branch struct{ key, text string }

// branches parses "key {text} key {text}".
func branches(s string) []branch {
	var out []branch
	i := 0
	for {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			return out
		}
		j := i
		for j < len(s) && s[j] != '{' && !isSpace(s[j]) {
			j++
		}
		key := s[i:j]
		for j < len(s) && isSpace(s[j]) {
			j++
		}
		if key == "" || j >= len(s) || s[j] != '{' {
			return nil
		}
		end := closing(s, j)
		if end < 0 {
			return nil
		}
		out = append(out, branch{key, s[j+1 : end]})
		i = end + 1
	}
}

func pick(bs []branch, keys ...string) string {
	for _, k := range keys {
		for _, b := range bs {
			if b.key == k {
				return b.text
			}
		}
	}
	return ""
}

// choice splits the tail of "{name, plural, …}" into its kind and branches.
func choice(rest string) (kind string, bs []branch) {
	r := strings.TrimLeft(rest, " \t\n\r")
	if !strings.HasPrefix(r, ",") {
		return "", nil
	}
	r = strings.TrimLeft(r[1:], " \t\n\r")
	for _, k := range []string{"plural", "select"} {
		if strings.HasPrefix(r, k) {
			tail := strings.TrimLeft(r[len(k):], " \t\n\r")
			if !strings.HasPrefix(tail, ",") {
				return "", nil
			}
			if bs := branches(tail[1:]); len(bs) > 0 {
				return k, bs
			}
		}
	}
	return "", nil
}

func valueString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case error:
		return x.Error()
	case fmt.Stringer:
		return x.String()
	}
	return fmt.Sprint(v)
}

func valueNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func format(msg string, params map[string]any, category func(float64) string, hash string, inPlural bool) string {
	var out strings.Builder
	for i := 0; i < len(msg); {
		c := msg[i]
		if c == '#' && inPlural {
			out.WriteString(hash)
			i++
			continue
		}
		if c != '{' {
			out.WriteByte(c)
			i++
			continue
		}
		end := closing(msg, i)
		name := ""
		if end >= 0 {
			name = ident(msg[i+1 : end])
		}
		if name == "" {
			out.WriteByte(c)
			i++
			continue
		}
		rest := msg[i+1+len(name) : end]
		value, has := params[name]
		if rest == "" {
			if has && value != nil {
				out.WriteString(valueString(value))
			} else {
				out.WriteString("{" + name + "}")
			}
			i = end + 1
			continue
		}
		kind, bs := choice(rest)
		switch kind {
		case "plural":
			n, _ := valueNumber(value)
			text := "0"
			if has && value != nil {
				text = valueString(value)
			}
			exact := "=" + strconv.FormatFloat(n, 'f', -1, 64)
			out.WriteString(format(pick(bs, exact, category(n), "other"), params, category, text, true))
		case "select":
			out.WriteString(format(pick(bs, valueString(value), "other"), params, category, hash, inPlural))
		default:
			out.WriteByte(c)
			i++
			continue
		}
		i = end + 1
	}
	return out.String()
}

// MessageParams lists the parameter names a message reads, for the catalogue
// checks.
func MessageParams(msg string) []string {
	seen := map[string]bool{}
	var walk func(string)
	walk = func(s string) {
		for i := 0; i < len(s); {
			if s[i] != '{' {
				i++
				continue
			}
			end := closing(s, i)
			name := ""
			if end >= 0 {
				name = ident(s[i+1 : end])
			}
			if name == "" {
				i++
				continue
			}
			rest := s[i+1+len(name) : end]
			if rest == "" {
				seen[name] = true
				i = end + 1
				continue
			}
			kind, bs := choice(rest)
			if kind == "" {
				i++
				continue
			}
			seen[name] = true
			for _, b := range bs {
				walk(b.text)
			}
			i = end + 1
		}
	}
	walk(msg)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
