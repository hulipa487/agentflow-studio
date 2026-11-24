import { t as i18n } from '@/i18n'
import {
  RuneText,
  SeverityError,
  isDigit,
  isIdentRune,
  isIdentStart,
  isSpace,
  keywords,
  type Diagnostic,
  type Token,
  type TokenKind,
} from './token'

/** A comment's code-point span. Comments are not tokens; the editor uses the spans to keep completion quiet inside them. */
export interface Span {
  start: number
  end: number
}

export interface LexResult {
  tokens: Token[]
  diagnostics: Diagnostic[]
  comments: Span[]
}

/**
 * Tokenizes TypeScript/JavaScript source. It never fails: malformed input
 * produces diagnostics and the longest sensible token so the editor can keep
 * linting while editing.
 */
export function lex(src: string | RuneText): LexResult {
  const l = new Lexer(typeof src === 'string' ? new RuneText(src) : src)
  l.run()
  return { tokens: l.tokens, diagnostics: l.diags, comments: l.comments }
}

const multiOps = [
  '>>>=',
  '>>>',
  '===',
  '!==',
  '**=',
  '??=',
  '&&=',
  '||=',
  '<<=',
  '>>=',
  '...',
  '=>',
  '?.',
  '??',
  '==',
  '!=',
  '<=',
  '>=',
  '&&',
  '||',
  '++',
  '--',
  '+=',
  '-=',
  '*=',
  '/=',
  '%=',
  '&=',
  '|=',
  '^=',
  '<<',
  '>>',
  '**',
]

const REGEX_AFTER_KEYWORDS = new Set([
  'return',
  'throw',
  'case',
  'delete',
  'void',
  'typeof',
  'in',
  'instanceof',
  'new',
  'else',
  'yield',
  'await',
  'do',
])

class Lexer {
  private readonly text: RuneText
  private readonly src: ArrayLike<string>
  private readonly n: number
  private i = 0
  tokens: Token[] = []
  diags: Diagnostic[] = []
  comments: Span[] = []

  constructor(text: RuneText) {
    this.text = text
    this.src = text.chars
    this.n = text.length
  }

  private peek(offset: number): string | undefined {
    const j = this.i + offset
    return j < this.n ? this.src[j] : undefined
  }

  private add(kind: TokenKind, start: number, end: number) {
    if (end <= start) return
    const sp = this.text.position(start)
    const ep = this.text.position(end)
    this.tokens.push({
      kind,
      lit: this.text.slice(start, end),
      line: sp.line,
      col: sp.col,
      endLine: ep.line,
      endCol: ep.col,
      start,
      end,
    })
  }

  private errorAt(start: number, end: number, code: string, message: string) {
    if (end <= start) end = start + 1
    if (end > this.n) end = this.n
    const sp = this.text.position(start)
    const ep = this.text.position(end)
    this.diags.push({
      line: sp.line,
      col: sp.col,
      endLine: ep.line,
      endCol: ep.col,
      severity: SeverityError,
      code,
      message,
    })
  }

  private canStartRegex(): boolean {
    if (this.tokens.length === 0) return true
    const prev = this.tokens[this.tokens.length - 1]
    if (prev.kind === 'keyword') return REGEX_AFTER_KEYWORDS.has(prev.lit)
    if (prev.kind === 'op') return prev.lit !== ')' && prev.lit !== ']' && prev.lit !== '++' && prev.lit !== '--'
    return false
  }

  run() {
    const src = this.src
    while (this.i < this.n) {
      const r = src[this.i]
      if (isSpace(r) || r === '\n') {
        this.i++
      } else if (r === '/' && this.peek(1) === '/') {
        this.skipLineComment()
      } else if (r === '/' && this.peek(1) === '*') {
        this.skipBlockComment()
      } else if (r === '"' || r === "'") {
        this.readString(r)
      } else if (r === '`') {
        this.readTemplateString()
      } else if (r === '/' && this.canStartRegex()) {
        this.readRegex()
      } else if (isDigit(r) || (r === '.' && isDigit(this.peek(1)))) {
        this.readNumber()
      } else if (isIdentStart(r)) {
        this.readName()
      } else {
        this.readOp()
      }
    }
    const endPos = this.text.position(this.n)
    this.tokens.push({
      kind: 'eof',
      lit: '',
      line: endPos.line,
      col: endPos.col,
      endLine: endPos.line,
      endCol: endPos.col,
      start: this.n,
      end: this.n,
    })
  }

  private skipLineComment() {
    const start = this.i
    this.i += 2
    while (this.i < this.n && this.src[this.i] !== '\n') this.i++
    this.comments.push({ start, end: this.i })
  }

  private skipBlockComment() {
    const start = this.i
    this.i += 2
    while (this.i < this.n) {
      if (this.src[this.i] === '*' && this.peek(1) === '/') {
        this.i += 2
        this.comments.push({ start, end: this.i })
        return
      }
      this.i++
    }
    this.comments.push({ start, end: this.n })
    this.errorAt(start, start + 2, 'syntax.unclosed-comment', i18n('script.diag.unclosedComment'))
  }

  private readString(quoteChar: string) {
    const start = this.i
    this.i++
    while (this.i < this.n) {
      const c = this.src[this.i]
      if (c === quoteChar) {
        this.i++
        this.add('string', start, this.i)
        return
      }
      if (c === '\n') break
      if (c === '\\') {
        this.i += 2
        continue
      }
      this.i++
    }
    this.add('string', start, this.i)
    this.errorAt(start, this.i, 'syntax.unclosed-string', i18n('script.diag.unfinishedString'))
  }

  private readTemplateString() {
    const start = this.i
    this.i++
    while (this.i < this.n) {
      const c = this.src[this.i]
      if (c === '`') {
        this.i++
        this.add('string', start, this.i)
        return
      }
      if (c === '\\') {
        this.i += 2
        continue
      }
      this.i++
    }
    this.add('string', start, this.i)
    this.errorAt(start, this.i, 'syntax.unclosed-string', i18n('script.diag.unfinishedString'))
  }

  private readRegex() {
    const start = this.i
    this.i++
    let inClass = false
    while (this.i < this.n) {
      const c = this.src[this.i]
      if (c === '\n') break
      if (c === '\\') {
        this.i += 2
        continue
      }
      if (c === '[') inClass = true
      else if (c === ']') inClass = false
      else if (c === '/' && !inClass) {
        this.i++
        while (this.i < this.n && isIdentRune(this.src[this.i])) this.i++
        this.add('string', start, this.i)
        return
      }
      this.i++
    }
    this.add('string', start, this.i)
    this.errorAt(start, this.i, 'syntax.unclosed-string', i18n('script.diag.unfinishedRegex'))
  }

  private readNumber() {
    const start = this.i
    if (this.src[this.i] === '0' && (this.peek(1) === 'x' || this.peek(1) === 'X')) {
      this.i += 2
      while (this.i < this.n && isHexOrSep(this.src[this.i])) this.i++
      this.add('number', start, this.i)
      return
    }
    if (this.src[this.i] === '0' && (this.peek(1) === 'b' || this.peek(1) === 'B')) {
      this.i += 2
      while (this.i < this.n && (this.src[this.i] === '0' || this.src[this.i] === '1' || this.src[this.i] === '_')) this.i++
      this.add('number', start, this.i)
      return
    }
    while (this.i < this.n && (isDigit(this.src[this.i]) || this.src[this.i] === '_')) this.i++
    if (this.src[this.i] === '.' && isDigit(this.peek(1))) {
      this.i++
      while (this.i < this.n && (isDigit(this.src[this.i]) || this.src[this.i] === '_')) this.i++
    }
    if (this.src[this.i] === 'e' || this.src[this.i] === 'E') {
      this.i++
      if (this.src[this.i] === '+' || this.src[this.i] === '-') this.i++
      while (this.i < this.n && (isDigit(this.src[this.i]) || this.src[this.i] === '_')) this.i++
    }
    this.add('number', start, this.i)
  }

  private readName() {
    const start = this.i
    while (this.i < this.n && isIdentRune(this.src[this.i])) this.i++
    const lit = this.text.slice(start, this.i)
    this.add(keywords.has(lit) ? 'keyword' : 'name', start, this.i)
  }

  private readOp() {
    const start = this.i
    for (const op of multiOps) {
      if (this.matchesAt(this.i, op)) {
        this.i += op.length
        this.add('op', start, this.i)
        return
      }
    }
    this.i++
    this.add('op', start, this.i)
  }

  private matchesAt(pos: number, needle: string): boolean {
    if (pos + needle.length > this.n) return false
    for (let k = 0; k < needle.length; k++) {
      if (this.src[pos + k] !== needle[k]) return false
    }
    return true
  }
}

function isHexOrSep(c: string | undefined): boolean {
  if (c === undefined || c.length !== 1) return false
  if (c === '_') return true
  const code = c.charCodeAt(0)
  return (code >= 48 && code <= 57) || (code >= 65 && code <= 70) || (code >= 97 && code <= 102)
}
