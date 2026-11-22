import { t as i18n } from '@/i18n'
import {
  RuneText,
  SeverityError,
  isDigit,
  isIdentRune,
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
 * Tokenizes Lua 5.1 source. It never fails: malformed input produces diagnostics
 * and the longest sensible token, so an editor can keep linting while the user
 * is mid-edit.
 */
export function lex(src: string | RuneText): LexResult {
  const l = new Lexer(typeof src === 'string' ? new RuneText(src) : src)
  l.run()
  return { tokens: l.tokens, diagnostics: l.diags, comments: l.comments }
}

const multiOps = [
  '...', '..=', '//=', '<<=', '>>=',
  '==', '~=', '<=', '>=', '::', '->',
  '+=', '-=', '*=', '/=', '%=', '^=',
  '..', '//', '<<', '>>', '&&', '||', '!=',
]

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

  run() {
    const src = this.src
    while (this.i < this.n) {
      const r = src[this.i]
      if (isSpace(r) || r === '\n') {
        this.i++
      } else if (r === '-' && this.peek(1) === '-') {
        this.skipComment()
      } else if (r === '"' || r === "'") {
        this.readString(r)
      } else if (r === '`') {
        this.readInterpolated()
      } else if (r === '[' && this.longBracketAt(this.i) >= 0) {
        this.readLongString()
      } else if (isDigit(r) || (r === '.' && isDigit(this.peek(1)))) {
        this.readNumber()
      } else if (isIdentRune(r)) {
        this.readName()
      } else {
        this.readOp()
      }
    }
    const sp = this.text.position(this.n)
    this.tokens.push({
      kind: 'eof',
      lit: '',
      line: sp.line,
      col: sp.col,
      endLine: sp.line,
      endCol: sp.col,
      start: this.n,
      end: this.n,
    })
  }

  private peek(n: number): string {
    if (this.i + n >= this.n) return ''
    return this.src[this.i + n]
  }

  /**
   * Returns the '=' count when src[at] starts a long bracket ("[[" or
   * "[=[" and so on), or -1.
   */
  private longBracketAt(at: number): number {
    const src = this.src
    if (at >= this.n || src[at] !== '[') return -1
    let j = at + 1
    while (j < this.n && src[j] === '=') j++
    if (j < this.n && src[j] === '[') return j - at - 1
    return -1
  }

  private skipComment() {
    const start = this.i
    this.i += 2
    const level = this.longBracketAt(this.i)
    if (level >= 0) {
      const open = this.i
      this.i += level + 2
      if (!this.skipLongBody(level)) {
        this.errorAt(open, this.n, 'syntax.unclosed-comment', i18n('lua.diag.unclosedComment'))
      }
      this.comments.push({ start, end: this.i })
      return
    }
    while (this.i < this.n && this.src[this.i] !== '\n') this.i++
    this.comments.push({ start, end: this.i })
  }

  /**
   * Advances past a long-bracket body and reports whether the closing bracket
   * was found. The opening bracket must already be consumed.
   */
  private skipLongBody(level: number): boolean {
    const src = this.src
    while (this.i < this.n) {
      if (src[this.i] === ']') {
        let j = this.i + 1
        for (let k = 0; k < level && j < this.n && src[j] === '='; k++) j++
        if (j < this.n && src[j] === ']' && j - this.i - 1 === level) {
          this.i = j + 1
          return true
        }
      }
      this.i++
    }
    return false
  }

  private readLongString() {
    const start = this.i
    const level = this.longBracketAt(this.i)
    this.i += level + 2
    if (!this.skipLongBody(level)) {
      this.errorAt(start, this.n, 'syntax.unclosed-string', i18n('lua.diag.unclosedLongString'))
      this.add('string', start, this.n)
      return
    }
    this.add('string', start, this.i)
  }

  private readString(quote: string) {
    const src = this.src
    const start = this.i
    this.i++
    while (this.i < this.n) {
      const r = src[this.i]
      if (r === '\\') {
        this.i++
        if (this.i < this.n) {
          // \z skips following whitespace, including newlines.
          if (src[this.i] === 'z') {
            this.i++
            while (this.i < this.n && (isSpace(src[this.i]) || src[this.i] === '\n')) this.i++
            continue
          }
          this.i++
        }
        continue
      }
      if (r === quote) {
        this.i++
        this.add('string', start, this.i)
        return
      }
      if (r === '\n') {
        this.errorAt(start, this.i, 'syntax.unclosed-string', i18n('lua.diag.unfinishedString'))
        this.add('string', start, this.i)
        return
      }
      this.i++
    }
    this.errorAt(start, this.n, 'syntax.unclosed-string', i18n('lua.diag.unfinishedString'))
    this.add('string', start, this.n)
  }

  /**
   * Treats a backtick string as one token. Expressions inside {} are
   * intentionally not linted; the string form is what the parser needs.
   */
  private readInterpolated() {
    const src = this.src
    const start = this.i
    this.i++
    while (this.i < this.n) {
      const r = src[this.i]
      if (r === '\\') {
        this.i += 2
        continue
      }
      if (r === '`') {
        this.i++
        this.errorAt(start, this.i, 'syntax.luau-only', i18n('lua.diag.luauInterpolated'))
        this.add('string', start, this.i)
        return
      }
      this.i++
    }
    // An escape as the last character steps one past the end.
    if (this.i > this.n) this.i = this.n
    this.errorAt(start, this.n, 'syntax.unclosed-string', i18n('lua.diag.unfinishedInterpolated'))
    this.add('string', start, this.n)
  }

  private readNumber() {
    const src = this.src
    const start = this.i
    while (this.i < this.n) {
      const r = src[this.i]
      if (isIdentRune(r) || r === '.') {
        this.i++
        continue
      }
      if ((r === '+' || r === '-') && this.i > start) {
        const prev = src[this.i - 1]
        if (prev === 'e' || prev === 'E' || prev === 'p' || prev === 'P') {
          this.i++
          continue
        }
      }
      break
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
    const src = this.src
    const start = this.i
    for (const op of multiOps) {
      if (start + op.length > this.n) continue
      let match = true
      for (let k = 0; k < op.length; k++) {
        if (src[start + k] !== op[k]) {
          match = false
          break
        }
      }
      if (match) {
        this.i += op.length
        this.add('op', start, this.i)
        return
      }
    }
    this.i++
    this.add('op', start, this.i)
  }
}
