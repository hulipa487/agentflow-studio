package lua

import (
	"strings"

	"agentflow-studio/internal/i18n"
)

// Lex tokenizes Luau source. It never fails: malformed input produces
// diagnostics and the longest sensible token, so an editor can keep linting
// while the user is mid-edit.
func Lex(src string) ([]Token, []Diagnostic) {
	l := &lexer{src: []rune(src), starts: lineStarts([]rune(src))}
	l.run()
	return l.tokens, l.diags
}

type lexer struct {
	src    []rune
	starts []int
	i      int
	tokens []Token
	diags  []Diagnostic
}

func (l *lexer) pos(off int) Position { return offsetToPosition(l.starts, off) }

func (l *lexer) add(kind TokenKind, start, end int) {
	if end <= start {
		return
	}
	sp, ep := l.pos(start), l.pos(end)
	l.tokens = append(l.tokens, Token{
		Kind: kind, Lit: string(l.src[start:end]),
		Line: sp.Line, Col: sp.Col, EndLine: ep.Line, EndCol: ep.Col,
		Start: start, End: end,
	})
}

func (l *lexer) errorAt(start, end int, code, msg string) {
	if end <= start {
		end = start + 1
	}
	if end > len(l.src) {
		end = len(l.src)
	}
	sp, ep := l.pos(start), l.pos(end)
	l.diags = append(l.diags, Diagnostic{
		Line: sp.Line, Col: sp.Col, EndLine: ep.Line, EndCol: ep.Col,
		Severity: SeverityError, Code: code, Message: msg,
	})
}

func (l *lexer) run() {
	for l.i < len(l.src) {
		r := l.src[l.i]
		switch {
		case isSpace(r) || r == '\n':
			l.i++
		case r == '-' && l.peek(1) == '-':
			l.skipComment()
		case r == '"' || r == '\'':
			l.readString(r)
		case r == '`':
			l.readInterpolated()
		case r == '[' && l.longBracketAt(l.i) >= 0:
			l.readLongString()
		case isDigit(r) || (r == '.' && isDigit(l.peek(1))):
			l.readNumber()
		case isIdentRune(r):
			l.readName()
		default:
			l.readOp()
		}
	}
	sp := l.pos(len(l.src))
	l.tokens = append(l.tokens, Token{
		Kind: TokenEOF, Line: sp.Line, Col: sp.Col,
		EndLine: sp.Line, EndCol: sp.Col, Start: len(l.src), End: len(l.src),
	})
}

func (l *lexer) peek(n int) rune {
	if l.i+n >= len(l.src) {
		return 0
	}
	return l.src[l.i+n]
}

// longBracketAt returns the '=' count when src[at] starts a long bracket
// ("[", "[[" or "[=[["), or -1.
func (l *lexer) longBracketAt(at int) int {
	if at >= len(l.src) || l.src[at] != '[' {
		return -1
	}
	j := at + 1
	for j < len(l.src) && l.src[j] == '=' {
		j++
	}
	if j < len(l.src) && l.src[j] == '[' {
		return j - at - 1
	}
	return -1
}

func (l *lexer) skipComment() {
	start := l.i
	l.i += 2
	if level := l.longBracketAt(l.i); level >= 0 {
		open := l.i
		l.i += level + 2
		if !l.skipLongBody(level) {
			l.errorAt(open, len(l.src), "syntax.unclosed-comment", i18n.T("lua.diag.unclosedComment"))
		}
		return
	}
	for l.i < len(l.src) && l.src[l.i] != '\n' {
		l.i++
	}
	_ = start
}

// skipLongBody advances past a long-bracket body and reports whether the
// closing bracket was found. The opening bracket must already be consumed.
func (l *lexer) skipLongBody(level int) bool {
	for l.i < len(l.src) {
		if l.src[l.i] == ']' {
			j := l.i + 1
			for k := 0; k < level && j < len(l.src) && l.src[j] == '='; k++ {
				j++
			}
			if j < len(l.src) && l.src[j] == ']' && j-l.i-1 == level {
				l.i = j + 1
				return true
			}
		}
		l.i++
	}
	return false
}

func (l *lexer) readLongString() {
	start := l.i
	level := l.longBracketAt(l.i)
	l.i += level + 2
	if !l.skipLongBody(level) {
		l.errorAt(start, len(l.src), "syntax.unclosed-string", i18n.T("lua.diag.unclosedLongString"))
		l.add(TokenString, start, len(l.src))
		return
	}
	l.add(TokenString, start, l.i)
}

func (l *lexer) readString(quote rune) {
	start := l.i
	l.i++
	for l.i < len(l.src) {
		r := l.src[l.i]
		if r == '\\' {
			l.i++
			if l.i < len(l.src) {
				// \z skips following whitespace, including newlines.
				if l.src[l.i] == 'z' {
					l.i++
					for l.i < len(l.src) && isSpace(l.src[l.i]) || (l.i < len(l.src) && l.src[l.i] == '\n') {
						l.i++
					}
					continue
				}
				l.i++
			}
			continue
		}
		if r == quote {
			l.i++
			l.add(TokenString, start, l.i)
			return
		}
		if r == '\n' {
			l.errorAt(start, l.i, "syntax.unclosed-string", i18n.T("lua.diag.unfinishedString"))
			l.add(TokenString, start, l.i)
			return
		}
		l.i++
	}
	l.errorAt(start, len(l.src), "syntax.unclosed-string", i18n.T("lua.diag.unfinishedString"))
	l.add(TokenString, start, len(l.src))
}

// readInterpolated treats a backtick string as one token. Expressions inside
// {} are intentionally not linted; the string form is what the parser needs.
func (l *lexer) readInterpolated() {
	start := l.i
	l.i++
	for l.i < len(l.src) {
		r := l.src[l.i]
		if r == '\\' {
			l.i += 2
			continue
		}
		if r == '`' {
			l.i++
			l.add(TokenString, start, l.i)
			return
		}
		l.i++
	}
	l.errorAt(start, len(l.src), "syntax.unclosed-string", i18n.T("lua.diag.unfinishedInterpolated"))
	l.add(TokenString, start, len(l.src))
}

func (l *lexer) readNumber() {
	start := l.i
	for l.i < len(l.src) {
		r := l.src[l.i]
		if isIdentRune(r) || r == '.' {
			l.i++
			continue
		}
		if (r == '+' || r == '-') && l.i > start {
			prev := l.src[l.i-1]
			if prev == 'e' || prev == 'E' || prev == 'p' || prev == 'P' {
				l.i++
				continue
			}
		}
		break
	}
	l.add(TokenNumber, start, l.i)
}

func (l *lexer) readName() {
	start := l.i
	for l.i < len(l.src) && isIdentRune(l.src[l.i]) {
		l.i++
	}
	lit := string(l.src[start:l.i])
	kind := TokenName
	if keywords[lit] {
		kind = TokenKeyword
	}
	l.add(kind, start, l.i)
}

var multiOps = []string{
	"...", "..=", "//=", "<<=", ">>=",
	"==", "~=", "<=", ">=", "::", "->",
	"+=", "-=", "*=", "/=", "%=", "^=",
	"..", "//", "<<", ">>", "&&", "||", "!=",
}

func (l *lexer) readOp() {
	start := l.i
	rest := string(l.src[l.i:])
	for _, op := range multiOps {
		if strings.HasPrefix(rest, op) {
			l.i += len([]rune(op))
			l.add(TokenOp, start, l.i)
			return
		}
	}
	l.i++
	l.add(TokenOp, start, l.i)
}
