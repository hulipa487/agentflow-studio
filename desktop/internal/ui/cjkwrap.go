package ui

import (
	"strings"
	"unicode"
)

// Fyne wraps text only where unicode.IsSpace allows, and Chinese is written
// without spaces: a sentence is one long "word" to it, so a paragraph breaks
// wherever a Latin word happens to end and leaves ragged half-empty lines.
// cjkBreaks gives it break points: a hair space (U+200A, about 0.8 px at body
// size in the CJK fonts) between two Chinese characters, except before
// closing punctuation and after opening punctuation, which must not start or
// end a line. Latin text and the edges between Latin and Chinese are left
// alone, so English words and identifiers never split.

const hairSpace = ' '

// noLineStart holds punctuation that must not begin a line.
const noLineStart = "，。、；：！？）」』》〉】〕…—·％,.;:!?)]}"

// noLineEnd holds punctuation that must not end a line.
const noLineEnd = "（「『《〈【〔([{"

func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		(r >= 0x3000 && r <= 0x303f) || // CJK symbols and punctuation
		(r >= 0xff00 && r <= 0xffef) || // full-width forms
		r == '…' || r == '—' || r == '·'
}

func cjkBreaks(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		return s
	}
	rs := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + len(rs))
	for i, r := range rs {
		if i > 0 {
			prev := rs[i-1]
			if isCJK(prev) && isCJK(r) && !strings.ContainsRune(noLineStart, r) && !strings.ContainsRune(noLineEnd, prev) {
				b.WriteRune(hairSpace)
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}
