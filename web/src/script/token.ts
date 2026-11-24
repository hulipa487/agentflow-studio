// AgentFlow's TypeScript/JS (Goja + Prelude v2) language service:
// lexing, diagnostics, completion and hover data for loop source.
//
// Positions are zero-based lines and Unicode code-point columns. CodeMirror
// and the public surface in index.ts work in UTF-16 offsets; RuneText converts
// between the two.

export type TokenKind = 'eof' | 'name' | 'number' | 'string' | 'keyword' | 'op'

/** One significant lexical unit. Comments and whitespace are skipped. */
export interface Token {
  kind: TokenKind
  lit: string
  /** line/col are the token's start; endLine/endCol are one past its end. */
  line: number
  col: number
  endLine: number
  endCol: number
  /** start/end are code-point offsets into the source. */
  start: number
  end: number
}

export const keywords: ReadonlySet<string> = new Set([
  'await',
  'break',
  'case',
  'catch',
  'class',
  'const',
  'continue',
  'debugger',
  'default',
  'delete',
  'do',
  'else',
  'enum',
  'export',
  'extends',
  'false',
  'finally',
  'for',
  'function',
  'if',
  'import',
  'in',
  'instanceof',
  'let',
  'new',
  'null',
  'return',
  'super',
  'switch',
  'this',
  'throw',
  'true',
  'try',
  'typeof',
  'var',
  'void',
  'while',
  'with',
  'yield',
])

/** Reports whether ch (one code point) can start or appear inside a JS identifier. */
export function isIdentStart(ch: string | undefined): boolean {
  if (ch === undefined || ch.length !== 1) return false
  const c = ch.charCodeAt(0)
  return c === 95 || c === 36 || (c >= 97 && c <= 122) || (c >= 65 && c <= 90)
}

export function isIdentRune(ch: string | undefined): boolean {
  if (ch === undefined || ch.length !== 1) return false
  const c = ch.charCodeAt(0)
  return c === 95 || c === 36 || (c >= 97 && c <= 122) || (c >= 65 && c <= 90) || (c >= 48 && c <= 57)
}

export function isDigit(ch: string | undefined): boolean {
  if (ch === undefined || ch.length !== 1) return false
  const c = ch.charCodeAt(0)
  return c >= 48 && c <= 57
}

export function isSpace(ch: string | undefined): boolean {
  return ch === ' ' || ch === '\t' || ch === '\r' || ch === '\v' || ch === '\f'
}

/** A zero-based line/column pair. */
export interface Position {
  line: number
  col: number
}

/** Reports whether p sorts before other. */
export function before(p: Position, other: Position): boolean {
  if (p.line !== other.line) return p.line < other.line
  return p.col < other.col
}

/** Mirrors the LSP diagnostic severity values. */
export const SeverityError = 1
export const SeverityWarning = 2
export const SeverityInfo = 3
export const SeverityHint = 4
export type SeverityLevel = 1 | 2 | 3 | 4

/** One finding in a loop source, in line/column terms. */
export interface Diagnostic {
  line: number
  col: number
  endLine: number
  endCol: number
  severity: SeverityLevel
  code: string
  message: string
  capability?: string
}

/**
 * RuneText is a source string addressed by code point. Sources without astral
 * characters are indexed in place; the code-point array and offset table are
 * only built when a surrogate is present.
 */
export class RuneText {
  readonly text: string
  /** One element per code point. A plain string when that is already true. */
  readonly chars: ArrayLike<string>
  readonly length: number
  /** The code-point offset of each line start. */
  readonly starts: number[]
  private readonly astral: boolean
  private utf16: Uint32Array | null = null

  constructor(text: string) {
    this.text = text
    this.astral = /[\uD800-\uDFFF]/.test(text)
    this.chars = this.astral ? Array.from(text) : text
    this.length = this.chars.length
    const starts = [0]
    for (let i = 0; i < this.length; i++) {
      if (this.chars[i] === '\n') starts.push(i + 1)
    }
    this.starts = starts
  }

  slice(start: number, end: number): string {
    if (!this.astral) return this.text.slice(start, end)
    return (this.chars as string[]).slice(start, end).join('')
  }

  /** Converts a code-point offset to a line/column pair. */
  position(off: number): Position {
    const starts = this.starts
    let lo = 0
    let hi = starts.length - 1
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1
      if (starts[mid] <= off) lo = mid
      else hi = mid - 1
    }
    return { line: lo, col: off - starts[lo] }
  }

  /**
   * Converts a line/column pair to a code-point offset, clamped to source end.
   */
  offset(line: number, col: number): number {
    if (line >= this.starts.length) return this.length
    const off = (line < 0 ? 0 : this.starts[line]) + col
    return off > this.length ? this.length : off
  }

  private table(): Uint32Array {
    if (this.utf16) return this.utf16
    const t = new Uint32Array(this.length + 1)
    let u = 0
    for (let i = 0; i < this.length; i++) {
      t[i] = u
      u += this.chars[i].length
    }
    t[this.length] = u
    this.utf16 = t
    return t
  }

  /** The UTF-16 offset (a JavaScript string index) of a code-point offset. */
  toUtf16(off: number): number {
    if (off <= 0) return 0
    if (off >= this.length) return this.text.length
    return this.astral ? this.table()[off] : off
  }

  /** The code-point offset of a UTF-16 offset; inside a pair it rounds down. */
  fromUtf16(index: number): number {
    if (index <= 0) return 0
    if (index >= this.text.length) return this.length
    if (!this.astral) return index
    const t = this.table()
    let lo = 0
    let hi = this.length
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1
      if (t[mid] <= index) lo = mid
      else hi = mid - 1
    }
    return lo
  }

  /** The number of code points on a line, without its newline. */
  lineLength(line: number): number {
    if (line < 0 || line >= this.starts.length) return 0
    const end = line + 1 < this.starts.length ? this.starts[line + 1] - 1 : this.length
    return end - this.starts[line]
  }
}

const goSpace = '\\t\\n\\v\\f\\r \\u0085\\u00A0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000'
const trimRE = new RegExp(`^[${goSpace}]+|[${goSpace}]+$`, 'g')

export function trimSpace(s: string): string {
  return s.replace(trimRE, '')
}

/** Formats s as a JSON-style double-quoted string. */
export function quote(s: string): string {
  let out = '"'
  for (const ch of s) {
    if (ch === '"' || ch === '\\') out += '\\' + ch
    else if (ch === '\n') out += '\\n'
    else if (ch === '\r') out += '\\r'
    else if (ch === '\t') out += '\\t'
    else out += ch
  }
  return out + '"'
}
