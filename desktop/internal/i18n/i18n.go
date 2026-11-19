// Package i18n is the desktop app's message layer: three catalogues, the
// current language, and T to read a message.
//
// The catalogues are the shared files in ../../../i18n (one JSON file per
// namespace, per language), the same ones the web app bundles. English is the
// source: a key missing from another language falls back to it, and a key
// missing everywhere prints as the key, so a typo is visible on screen.
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync/atomic"

	catalogues "agentflow-studio-i18n"
)

// Lang is a language Studio speaks.
type Lang string

const (
	En     Lang = "en"
	ZhHans Lang = "zh-Hans"
	ZhHant Lang = "zh-Hant"
)

// Langs is every language, in toggle order.
var Langs = []Lang{En, ZhHans, ZhHant}

// Label is what the toggle shows for a language: its own short name.
func (l Lang) Label() string {
	switch l {
	case ZhHans:
		return "简体"
	case ZhHant:
		return "繁體"
	}
	return "EN"
}

// Name is the language's full name, in that language.
func (l Lang) Name() string {
	switch l {
	case ZhHans:
		return "简体中文"
	case ZhHant:
		return "繁體中文"
	}
	return "English"
}

// Chinese reports whether text in this language needs a CJK font.
func (l Lang) Chinese() bool { return l == ZhHans || l == ZhHant }

// Parse returns the language with that id; ok is false for anything else.
func Parse(id string) (Lang, bool) {
	for _, l := range Langs {
		if string(l) == id {
			return l, true
		}
	}
	return En, false
}

var (
	messages = load(catalogues.FS)
	current  atomic.Value // Lang
)

// load reads every "<lang>/<namespace>.json" in fsys. The catalogues are
// compiled in, so a file that does not parse is a build mistake: it panics
// rather than starting an app with half its text.
func load(fsys fs.FS) map[Lang]map[string]string {
	out := map[Lang]map[string]string{}
	for _, l := range Langs {
		out[l] = map[string]string{}
		files, err := fs.Glob(fsys, path.Join(string(l), "*.json"))
		if err != nil {
			panic(err)
		}
		for _, name := range files {
			b, err := fs.ReadFile(fsys, name)
			if err != nil {
				panic(err)
			}
			var m map[string]string
			if err := json.Unmarshal(b, &m); err != nil {
				panic(fmt.Sprintf("i18n: %s: %v", name, err))
			}
			for k, v := range m {
				out[l][k] = v
			}
		}
	}
	return out
}

// Language returns the current language; English until SetLanguage is called.
func Language() Lang {
	if l, ok := current.Load().(Lang); ok {
		return l
	}
	return En
}

// SetLanguage switches the language every later T reads.
func SetLanguage(l Lang) {
	if _, ok := messages[l]; !ok {
		l = En
	}
	current.Store(l)
}

// Match returns the language for a locale such as "zh_HK.UTF-8" or "zh-Hant-TW";
// ok is false when Studio has none for it. Traditional Chinese is the script
// tag Hant or, without a script, the regions that write it (Taiwan, Hong Kong,
// Macau); any other Chinese is Simplified.
func Match(locale string) (Lang, bool) {
	s := strings.ToLower(locale)
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return En, false
	}
	switch parts[0] {
	case "en":
		return En, true
	case "zh":
	default:
		return En, false
	}
	has := func(want ...string) bool {
		for _, p := range parts[1:] {
			for _, w := range want {
				if p == w {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("hant"):
		return ZhHant, true
	case has("hans"):
		return ZhHans, true
	case has("tw", "hk", "mo"):
		return ZhHant, true
	}
	return ZhHans, true
}

// Detect returns the first of the preferred locales Studio speaks, else
// English.
func Detect(preferred ...string) Lang {
	for _, loc := range preferred {
		if l, ok := Match(loc); ok {
			return l
		}
	}
	return En
}

func category(l Lang) func(float64) string {
	if l == En {
		return func(n float64) string {
			if n == 1 {
				return "one"
			}
			return "other"
		}
	}
	return func(float64) string { return "other" }
}

// Message returns the raw message for key in l, falling back to English and
// then to the key.
func Message(l Lang, key string) string {
	if m, ok := messages[l][key]; ok {
		return m
	}
	if m, ok := messages[En][key]; ok {
		return m
	}
	return key
}

// Has reports whether a key exists (in English, the source).
func Has(key string) bool {
	_, ok := messages[En][key]
	return ok
}

// Translate formats key in l. Parameters are name, value pairs:
//
//	i18n.T("common.required", "what", name)
func Translate(l Lang, key string, args ...any) string {
	msg := Message(l, key)
	if len(args) == 0 && !strings.Contains(msg, "{") {
		return msg
	}
	params := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		if name, ok := args[i].(string); ok {
			params[name] = args[i+1]
		}
	}
	return format(msg, params, category(l), "", false)
}

// T formats key in the current language.
func T(key string, args ...any) string {
	return Translate(Language(), key, args...)
}

// Catalogue returns a copy of one language's messages, for the checks.
func Catalogue(l Lang) map[string]string {
	out := make(map[string]string, len(messages[l]))
	for k, v := range messages[l] {
		out[k] = v
	}
	return out
}
