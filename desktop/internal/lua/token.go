// Package lua implements the AgentFlow Luau dialect's language service:
// lexing, diagnostics, completion and hover data for loop source.
//
// Positions are zero-based lines and Unicode code-point columns (the LSP
// utf-32 position encoding). The editor works in the same units, so no
// conversion is needed between the LSP and Fyne's cursor.
package lua

import "strings"

// TokenKind classifies a lexed token.
type TokenKind int

const (
	TokenEOF TokenKind = iota
	TokenName
	TokenNumber
	TokenString
	TokenKeyword
	TokenOp
)

// Token is one significant lexical unit. Comments and whitespace are skipped.
type Token struct {
	Kind TokenKind
	Lit  string
	// Line/Col are the token's start; EndLine/EndCol are one past its end.
	Line, Col       int
	EndLine, EndCol int
	// Start/End are rune offsets into the source.
	Start, End int
}

func (t Token) Is(lit string) bool { return t.Lit == lit }

func (t Token) isName() bool { return t.Kind == TokenName }

var keywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true,
	"end": true, "false": true, "for": true, "function": true, "if": true,
	"in": true, "local": true, "nil": true, "not": true, "or": true,
	"repeat": true, "return": true, "then": true, "true": true,
	"until": true, "while": true,
	// Luau additions.
	"continue": true, "type": true, "export": true,
}

// isIdentRune reports whether r can appear in a name.
func isIdentRune(r rune) bool {
	return r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\v' || r == '\f'
}

// Position is a zero-based line/column pair.
type Position struct {
	Line, Col int
}

// Before reports whether p sorts before other.
func (p Position) Before(other Position) bool {
	if p.Line != other.Line {
		return p.Line < other.Line
	}
	return p.Col < other.Col
}

// Severity mirrors the LSP diagnostic severity values.
type Severity int

const (
	SeverityError   Severity = 1
	SeverityWarning Severity = 2
	SeverityInfo    Severity = 3
	SeverityHint    Severity = 4
)

// Diagnostic is one finding in a loop source.
type Diagnostic struct {
	Line, Col       int
	EndLine, EndCol int
	Severity        Severity
	Code            string
	Message         string
}

func (d Diagnostic) Start() Position { return Position{d.Line, d.Col} }
func (d Diagnostic) End() Position   { return Position{d.EndLine, d.EndCol} }

// lineStarts returns the rune offset of each line start.
func lineStarts(src []rune) []int {
	starts := []int{0}
	for i, r := range src {
		if r == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// offsetToPosition converts a rune offset to a line/column pair.
func offsetToPosition(starts []int, off int) Position {
	lo, hi := 0, len(starts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if starts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return Position{Line: lo, Col: off - starts[lo]}
}

func firstRune(s string) string {
	if s == "" {
		return ""
	}
	return string([]rune(s)[0])
}

func trimComment(s string) string { return strings.TrimSpace(strings.TrimPrefix(s, "--")) }
