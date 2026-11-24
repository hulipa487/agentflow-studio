import { t as i18n } from '@/i18n'
import {
  fieldNames,
  globalNames,
  isGlobalName,
  isKnownGlobal,
  lookup,
  stdlibGlobals,
  suggest,
  unavailableGlobals,
} from './api'
import { lex, type Span } from './lexer'
import {
  RuneText,
  SeverityError,
  SeverityInfo,
  SeverityWarning,
  quote,
  trimSpace,
  type Diagnostic,
  type Position,
  type SeverityLevel,
  type Token,
} from './token'

/** What the analyzer knows about the deployment and the agent. */
export interface AnalysisContext {
  /**
   * The runtime prelude version. Zero defaults to checking against version 2
   * while still requiring the directive.
   */
  preludeVersion: number
  /**
   * The agent's effective capability list. null/undefined skips capability checks;
   * an empty list flags every capability-gated call.
   */
  capabilities?: readonly string[] | null
  /**
   * Whether the chunk must declare a top-level `loop` function. True by default;
   * false for support chunks, shared packages, and safety filters.
   */
  requireLoop?: boolean
}

export type LocalKind = 'local' | 'param' | 'loop' | 'function'

/** A name declared inside the chunk, with the scope it lives in. */
export interface Local {
  name: string
  kind: LocalKind
  start: Position
  /** {0, 0} if unclosed, otherwise closing brace or EOF position. */
  end: Position
  /** The position of the declaring identifier. */
  decl: Position
}

/** The reusable result of an analysis pass. */
export interface Model {
  source: string
  text: RuneText
  tokens: Token[]
  /** Comment spans, in code-point offsets. */
  comments: Span[]
  locals: Local[]
  globals: Set<string>
}

/** A full analysis of one document. */
export interface AnalysisResult {
  diagnostics: Diagnostic[]
  model: Model
}

const TS_TYPE_WORDS = new Set([
  'any',
  'unknown',
  'never',
  'void',
  'string',
  'number',
  'boolean',
  'object',
  'symbol',
  'bigint',
  'Record',
  'Array',
  'Partial',
  'Required',
  'Readonly',
  'Pick',
  'Omit',
  'Promise',
  'Iterable',
  'Message',
  'ChatMessage',
  'ChatResponse',
  'ToolCallSpec',
  'ToolSpec',
  'HTTPRequest',
  'HTTPResponse',
  'ExtHost',
  'ExtAction',
  'APIDescriptor',
  'BatchAPICall',
  'BatchAPIResult',
  'SafetyIngressInput',
  'SafetyIngressResult',
  'SafetyEgressInput',
  'SafetyEgressResult',
])

const CONTROL_HEADERS = new Set(['if', 'for', 'while', 'switch', 'catch', 'with'])
const DIRECTIVE_RE = /^\s*\/\/\s*af-prelude-version\s*:\s*(.*)$/i
const IMPORT_RE = /^\s*\/\/\s*af-import\s*:/i

interface HeaderInfo {
  diags: Diagnostic[]
  hasImport: boolean
}

function scanLeadingDirectives(src: string, wantVersion: number): HeaderInfo {
  const diags: Diagnostic[] = []
  const want = wantVersion > 0 ? wantVersion : 2
  const lines = src.split('\n')
  const headers: { line: number; col: number; endCol: number; valid: boolean; version: number }[] = []
  let hasImport = false
  let inBlockComment = false

  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i].replace(/\r$/, '')
    const trimmed = trimSpace(raw)
    if (inBlockComment) {
      const closeIdx = trimmed.indexOf('*/')
      if (closeIdx < 0) continue
      inBlockComment = false
      const after = trimSpace(trimmed.slice(closeIdx + 2))
      if (after === '' || after.startsWith('//')) {
        if (IMPORT_RE.test(after)) hasImport = true
        continue
      }
      break
    }
    if (trimmed === '') continue
    if (trimmed.startsWith('/*')) {
      const closeIdx = trimmed.indexOf('*/', 2)
      if (closeIdx < 0) {
        inBlockComment = true
        continue
      }
      const after = trimSpace(trimmed.slice(closeIdx + 2))
      if (after === '' || after.startsWith('//')) {
        if (IMPORT_RE.test(after)) hasImport = true
        continue
      }
      break
    }
    if (trimmed.startsWith('//')) {
      if (IMPORT_RE.test(trimmed)) hasImport = true
      const m = DIRECTIVE_RE.exec(trimmed)
      if (m) {
        const num = Number.parseInt(trimSpace(m[1]), 10)
        const lead = raw.length - raw.trimStart().length
        headers.push({
          line: i,
          col: lead,
          endCol: raw.length,
          valid: /^\d+$/.test(trimSpace(m[1])) && num > 0,
          version: Number.isFinite(num) ? num : 0,
        })
      }
      continue
    }
    break
  }

  if (headers.length === 0) {
    const firstNonEmpty = lines.findIndex((l) => trimSpace(l) !== '')
    const line = firstNonEmpty >= 0 ? firstNonEmpty : 0
    diags.push({
      line,
      col: 0,
      endLine: line,
      endCol: Math.min(1, lines[line]?.length ?? 0),
      severity: SeverityError,
      code: 'prelude.missing-directive',
      message: i18n('script.diag.missingDirective', { want }),
    })
    return { diags, hasImport }
  }

  if (headers.length > 1) {
    for (let k = 1; k < headers.length; k++) {
      const h = headers[k]
      diags.push({
        line: h.line,
        col: h.col,
        endLine: h.line,
        endCol: h.endCol,
        severity: SeverityError,
        code: 'prelude.duplicate-directive',
        message: i18n('script.diag.duplicateDirective'),
      })
    }
  }

  const h = headers[0]
  if (!h.valid || h.version !== want) {
    diags.push({
      line: h.line,
      col: h.col,
      endLine: h.line,
      endCol: h.endCol,
      severity: SeverityError,
      code: 'prelude.version-mismatch',
      message: i18n('script.diag.versionMismatch', { got: h.version, want }),
    })
  }
  return { diags, hasImport }
}

function checkStructure(tokens: Token[]): { diags: Diagnostic[]; braceEnds: Map<number, Position> } {
  const diags: Diagnostic[] = []
  const braceEnds = new Map<number, Position>()
  const stack: { open: string; want: string; tokenIndex: number; tok: Token }[] = []
  const pairs: Record<string, string> = { '(': ')', '{': '}', '[': ']' }
  const closers: Record<string, string> = { ')': '(', '}': '{', ']': '[' }

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    if (t.kind !== 'op') continue
    const close = pairs[t.lit]
    if (close) {
      stack.push({ open: t.lit, want: close, tokenIndex: i, tok: t })
      continue
    }
    const open = closers[t.lit]
    if (!open) continue
    const matchIdx = stack.findLastIndex((entry) => entry.want === t.lit)
    if (matchIdx < 0) {
      if (stack.length === 0) {
        diags.push({
          line: t.line,
          col: t.col,
          endLine: t.endLine,
          endCol: t.endCol,
          severity: SeverityError,
          code: 'syntax.unexpected-close',
          message: i18n('script.diag.unexpectedClose', { token: quote(t.lit) }),
        })
      } else {
        const top = stack[stack.length - 1]
        diags.push({
          line: t.line,
          col: t.col,
          endLine: t.endLine,
          endCol: t.endCol,
          severity: SeverityError,
          code: 'syntax.mismatched-close',
          message: i18n('script.diag.mismatchedClose', { token: quote(t.lit), open: quote(top.open) }),
        })
      }
      continue
    }
    while (stack.length - 1 > matchIdx) {
      const unclosed = stack.pop()!
      diags.push({
        line: unclosed.tok.line,
        col: unclosed.tok.col,
        endLine: unclosed.tok.endLine,
        endCol: unclosed.tok.endCol,
        severity: SeverityError,
        code: 'syntax.unclosed-bracket',
        message: i18n('script.diag.unclosedBracket', { token: quote(unclosed.open) }),
      })
    }
    const top = stack.pop()!
    if (top.open === '{') {
      braceEnds.set(top.tokenIndex, { line: t.endLine, col: t.endCol })
    }
  }

  for (const unclosed of stack) {
    diags.push({
      line: unclosed.tok.line,
      col: unclosed.tok.col,
      endLine: unclosed.tok.endLine,
      endCol: unclosed.tok.endCol,
      severity: SeverityError,
      code: 'syntax.unclosed-bracket',
      message: i18n('script.diag.unclosedBracket', { token: quote(unclosed.open) }),
    })
  }
  return { diags, braceEnds }
}

/** Lints one TypeScript/JS loop or support chunk. */
export function analyzeSource(src: string, ctx: AnalysisContext): AnalysisResult {
  const text = new RuneText(src)
  const { tokens, diagnostics: lexDiags, comments } = lex(text)

  const header = scanLeadingDirectives(src, ctx.preludeVersion)
  const { diags: structDiags, braceEnds } = checkStructure(tokens)

  const model: Model = {
    source: src,
    text,
    tokens,
    comments,
    locals: [],
    globals: new Set(),
  }

  const analyzer = new Analyzer(tokens, braceEnds, header.hasImport, ctx, model)
  analyzer.collectDeclarations()
  analyzer.analyzeRefs()

  let diags = [...lexDiags, ...header.diags, ...structDiags, ...analyzer.diags]
  if (ctx.requireLoop !== false) {
    diags.push(...analyzer.entryPointDiagnostics())
  }
  diags = dedupeDiagnostics(diags)
  diags.sort((a, b) => {
    if (a.line !== b.line) return a.line - b.line
    if (a.col !== b.col) return a.col - b.col
    return a.code.localeCompare(b.code)
  })
  return { diagnostics: diags, model }
}

function dedupeDiagnostics(list: Diagnostic[]): Diagnostic[] {
  const seen = new Set<string>()
  const out: Diagnostic[] = []
  for (const d of list) {
    const key = `${d.line}:${d.col}:${d.code}:${d.message}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push(d)
  }
  return out
}

class Analyzer {
  private readonly tokens: Token[]
  private readonly braceEnds: Map<number, Position>
  private readonly hasImport: boolean
  private readonly ctx: AnalysisContext
  private readonly model: Model
  private readonly eofPos: Position
  private topLevelLoop = false
  private nestedLoopTok: Token | null = null
  private readonly typeNames = new Set<string>(TS_TYPE_WORDS)
  private readonly declTokenIndices = new Set<number>()
  private readonly funcBraces = new Set<number>()
  private readonly loopBodyBraces = new Set<number>()
  diags: Diagnostic[] = []

  constructor(
    tokens: Token[],
    braceEnds: Map<number, Position>,
    hasImport: boolean,
    ctx: AnalysisContext,
    model: Model,
  ) {
    this.tokens = tokens
    this.braceEnds = braceEnds
    this.hasImport = hasImport
    this.ctx = ctx
    this.model = model
    const last = tokens[tokens.length - 1]
    this.eofPos = last ? { line: last.line, col: last.col } : { line: 0, col: 0 }
  }

  private diag(tok: Token, severity: SeverityLevel, code: string, message: string, capability?: string) {
    this.diags.push({
      line: tok.line,
      col: tok.col,
      endLine: tok.endLine,
      endCol: tok.endCol,
      severity,
      code,
      message,
      ...(capability ? { capability } : {}),
    })
  }

  /** First pass: collect globals, locals with block scopes, function braces, and TS types. */
  collectDeclarations() {
    const toks = this.tokens
    const scopeStack: Position[] = [this.eofPos]
    let pendingFuncScopeEnd: Position | null = null
    let nextLoopHeaderEnd: Position | null = null

    for (let i = 0; i < toks.length; i++) {
      const t = toks[i]
      if (t.kind === 'eof') break

      if (t.kind === 'op' && t.lit === '{') {
        const endPos = this.braceEnds.get(i) ?? this.eofPos
        const pushEnd = pendingFuncScopeEnd ?? nextLoopHeaderEnd ?? endPos
        pendingFuncScopeEnd = null
        nextLoopHeaderEnd = null
        scopeStack.push(pushEnd)
        continue
      }
      if (t.kind === 'op' && t.lit === '}') {
        if (scopeStack.length > 1) scopeStack.pop()
        continue
      }

      const depth = scopeStack.length - 1
      const currentEnd = scopeStack[scopeStack.length - 1]

      // TS `type Foo = ...` or `interface Foo { ... }` declarations.
      if (
        t.kind === 'name' &&
        (t.lit === 'type' || t.lit === 'interface') &&
        (i === 0 || toks[i - 1].lit !== '.') &&
        toks[i + 1]?.kind === 'name'
      ) {
        this.typeNames.add(toks[i + 1].lit)
        this.declTokenIndices.add(i)
        this.declTokenIndices.add(i + 1)
        this.markTypeDeclTokens(i + 2, t.lit === 'interface')
        continue
      }

      // `for (const x of ...)` / `for (let i = 0; ...)`
      if (t.kind === 'keyword' && t.lit === 'for' && toks[i + 1]?.lit === '(') {
        const close = this.findMatchingParen(i + 1)
        const bodyBrace = close >= 0 && toks[close + 1]?.lit === '{' ? close + 1 : -1
        if (bodyBrace >= 0) nextLoopHeaderEnd = this.braceEnds.get(bodyBrace) ?? this.eofPos
        if (close > i + 1) {
          let pDepth = 0
          let bDepth = 0
          let brDepth = 0
          for (let k = i + 2; k < close; k++) {
            const kt = toks[k]
            if (kt.lit === '(') pDepth++
            else if (kt.lit === ')') pDepth = Math.max(0, pDepth - 1)
            else if (kt.lit === '{') bDepth++
            else if (kt.lit === '}') bDepth = Math.max(0, bDepth - 1)
            else if (kt.lit === '[') brDepth++
            else if (kt.lit === ']') brDepth = Math.max(0, brDepth - 1)
            else if (pDepth === 0 && bDepth === 0 && brDepth === 0 && kt.kind === 'name' && kt.lit === 'of') {
              this.declTokenIndices.add(k)
            }
          }
        }
        continue
      }

      // `function name(a, b) { ... }` or `function (a, b) { ... }`
      if (t.kind === 'keyword' && t.lit === 'function') {
        let j = i + 1
        let funcName: Token | null = null
        if (toks[j]?.kind === 'name') {
          funcName = toks[j]
          this.declTokenIndices.add(j)
          if (funcName.lit === 'loop') {
            if (depth === 0) this.topLevelLoop = true
            else if (!this.nestedLoopTok) this.nestedLoopTok = funcName
          }
          if (depth === 0) {
            this.model.globals.add(funcName.lit)
          } else {
            this.model.locals.push({
              name: funcName.lit,
              kind: 'function',
              start: { line: funcName.line, col: funcName.col },
              end: currentEnd,
              decl: { line: funcName.line, col: funcName.col },
            })
          }
          j++
        }
        if (toks[j]?.lit === '<') j = this.skipAngles(j)
        if (toks[j]?.lit === '(') {
          const closeParen = this.findMatchingParen(j)
          const bodyOpen = closeParen >= 0 ? this.findNextOpenBrace(closeParen + 1) : -1
          if (bodyOpen >= 0) {
            this.funcBraces.add(bodyOpen)
            if (funcName?.lit === 'loop' && depth === 0) this.loopBodyBraces.add(bodyOpen)
          }
          const bodyEnd = bodyOpen >= 0 ? (this.braceEnds.get(bodyOpen) ?? this.eofPos) : currentEnd
          pendingFuncScopeEnd = bodyEnd
          this.collectParamNames(j + 1, closeParen, 'param', bodyEnd)
        }
        continue
      }

      // Arrow functions: `(a, b) => ...` or `x => ...`
      if (t.kind === 'op' && t.lit === '=>') {
        const nextTok = toks[i + 1]
        if (nextTok?.lit === '{') {
          this.funcBraces.add(i + 1)
          const bodyEnd = this.braceEnds.get(i + 1) ?? currentEnd
          pendingFuncScopeEnd = bodyEnd
          this.collectArrowParamsBefore(i, bodyEnd)
        } else {
          // Single-expression arrow body: scope covers the arrow expression line/statement.
          const exprEnd = { line: t.line + 2, col: 0 }
          this.collectArrowParamsBefore(i, exprEnd)
        }
        continue
      }

      // Object method shorthand `{ handler(args) { ... } }`
      if (
        t.kind === 'name' &&
        (toks[i - 1]?.lit === '{' || toks[i - 1]?.lit === ',') &&
        !CONTROL_HEADERS.has(t.lit) &&
        toks[i + 1]?.lit === '('
      ) {
        const closeParen = this.findMatchingParen(i + 1)
        const bodyOpen = closeParen >= 0 && toks[closeParen + 1]?.lit === '{' ? closeParen + 1 : -1
        if (bodyOpen >= 0) {
          this.declTokenIndices.add(i)
          this.funcBraces.add(bodyOpen)
          const bodyEnd = this.braceEnds.get(bodyOpen) ?? currentEnd
          pendingFuncScopeEnd = bodyEnd
          this.collectParamNames(i + 2, closeParen, 'param', bodyEnd)
        }
        continue
      }

      // `catch (err)`
      if (t.kind === 'keyword' && t.lit === 'catch' && toks[i + 1]?.lit === '(' && toks[i + 2]?.kind === 'name') {
        const paramTok = toks[i + 2]
        const brace = this.findNextOpenBrace(i + 3)
        const end = brace >= 0 ? (this.braceEnds.get(brace) ?? currentEnd) : currentEnd
        this.declTokenIndices.add(i + 2)
        this.model.locals.push({
          name: paramTok.lit,
          kind: 'local',
          start: { line: paramTok.line, col: paramTok.col },
          end,
          decl: { line: paramTok.line, col: paramTok.col },
        })
        continue
      }

      // `const | let | var ...`
      if (t.kind === 'keyword' && (t.lit === 'const' || t.lit === 'let' || t.lit === 'var')) {
        const declEnd = nextLoopHeaderEnd ?? currentEnd
        const kind: LocalKind = nextLoopHeaderEnd ? 'loop' : 'local'
        this.collectVarBindings(i + 1, kind, depth, declEnd)
        continue
      }

      // `loop = ...` or `globalThis.loop = ...` at top level
      if (depth === 0 && t.kind === 'name' && t.lit === 'loop' && toks[i + 1]?.lit === '=') {
        this.topLevelLoop = true
        this.model.globals.add('loop')
      }
      if (depth === 0 && t.kind === 'name' && t.lit === 'globalThis' && toks[i + 1]?.lit === '.' && toks[i + 2]?.lit === 'loop') {
        this.topLevelLoop = true
        this.model.globals.add('loop')
      }
    }
  }

  private collectArrowParamsBefore(arrowIdx: number, end: Position) {
    const toks = this.tokens
    if (arrowIdx < 1) return
    const prev = toks[arrowIdx - 1]
    if (prev.kind === 'name') {
      this.declTokenIndices.add(arrowIdx - 1)
      this.model.locals.push({
        name: prev.lit,
        kind: 'param',
        start: { line: prev.line, col: prev.col },
        end,
        decl: { line: prev.line, col: prev.col },
      })
      return
    }
    if (prev.lit !== ')') return
    const open = this.findMatchingParenBack(arrowIdx - 1)
    if (open >= 0) {
      this.collectParamNames(open + 1, arrowIdx - 1, 'param', end)
    }
  }

  private findMatchingParen(openIdx: number): number {
    let depth = 0
    for (let i = openIdx; i < this.tokens.length; i++) {
      const lit = this.tokens[i].lit
      if (lit === '(') depth++
      else if (lit === ')') {
        depth--
        if (depth === 0) return i
      }
    }
    return -1
  }

  private findMatchingParenBack(closeIdx: number): number {
    let depth = 0
    for (let i = closeIdx; i >= 0; i--) {
      const lit = this.tokens[i].lit
      if (lit === ')') depth++
      else if (lit === '(') {
        depth--
        if (depth === 0) return i
      }
    }
    return -1
  }

  private findNextOpenBrace(fromIdx: number): number {
    const toks = this.tokens
    if (toks[fromIdx]?.lit === ':') {
      let braceDepth = 0
      let parenDepth = 0
      let bracketDepth = 0
      let angleDepth = 0
      let expectAtom = true
      for (let i = fromIdx + 1; i < toks.length; i++) {
        const tok = toks[i]
        if (tok.kind === 'eof') return -1
        if (braceDepth === 0 && parenDepth === 0 && bracketDepth === 0 && angleDepth === 0) {
          if (tok.lit === '{') {
            if (!expectAtom) return i
            braceDepth++
            expectAtom = true
            continue
          }
          if (tok.lit === ';') return -1
        }
        if (tok.kind === 'name') this.declTokenIndices.add(i)
        if (tok.lit === '{') {
          braceDepth++
          expectAtom = true
        } else if (tok.lit === '}') {
          braceDepth = Math.max(0, braceDepth - 1)
          expectAtom = false
        } else if (tok.lit === '(') {
          parenDepth++
          expectAtom = true
        } else if (tok.lit === ')') {
          parenDepth = Math.max(0, parenDepth - 1)
          expectAtom = false
        } else if (tok.lit === '[') {
          bracketDepth++
          expectAtom = true
        } else if (tok.lit === ']') {
          bracketDepth = Math.max(0, bracketDepth - 1)
          expectAtom = false
        } else if (tok.lit === '<') {
          angleDepth++
          expectAtom = true
        } else if (tok.lit === '>') {
          angleDepth = Math.max(0, angleDepth - 1)
          expectAtom = false
        } else if (tok.lit === '|' || tok.lit === '&' || tok.lit === '=>') {
          expectAtom = true
        } else if (tok.kind === 'name' || tok.kind === 'keyword') {
          expectAtom = false
        }
      }
      return -1
    }
    for (let i = fromIdx; i < toks.length; i++) {
      const tok = toks[i]
      if (tok.lit === '{') return i
      if (tok.lit === ';') return -1
    }
    return -1
  }

  private markTypeDeclTokens(fromIdx: number, isInterface: boolean) {
    const toks = this.tokens
    let braceDepth = 0
    let parenDepth = 0
    let bracketDepth = 0
    let angleDepth = 0
    let inBody = false
    for (let i = fromIdx; i < toks.length; i++) {
      const t = toks[i]
      if (t.kind === 'eof') break
      if (t.lit === '{') {
        braceDepth++
        inBody = true
        continue
      }
      if (t.lit === '}') {
        braceDepth = Math.max(0, braceDepth - 1)
        if (isInterface && inBody && braceDepth === 0) break
        continue
      }
      if (t.lit === '(') parenDepth++
      else if (t.lit === ')') parenDepth = Math.max(0, parenDepth - 1)
      else if (t.lit === '[') bracketDepth++
      else if (t.lit === ']') bracketDepth = Math.max(0, bracketDepth - 1)
      else if (t.lit === '<') angleDepth++
      else if (t.lit === '>') angleDepth = Math.max(0, angleDepth - 1)

      if (braceDepth === 0 && parenDepth === 0 && bracketDepth === 0 && angleDepth === 0) {
        if (t.lit === ';') break
        if (!isInterface && t.kind === 'keyword' && t.lit !== 'null' && t.lit !== 'void') break
      }
      if (t.kind === 'name') this.declTokenIndices.add(i)
    }
  }

  private skipTypeExprAfterAs(fromIdx: number): number {
    const toks = this.tokens
    let i = fromIdx
    let parenDepth = 0
    let braceDepth = 0
    let bracketDepth = 0
    let angleDepth = 0
    let expectAtom = true
    while (i < toks.length) {
      const t = toks[i]
      if (t.kind === 'eof') break
      if (parenDepth === 0 && braceDepth === 0 && bracketDepth === 0 && angleDepth === 0) {
        if (t.lit === '{') {
          if (!expectAtom) break
          braceDepth++
          expectAtom = true
          i++
          continue
        }
        if (t.lit === '(') {
          if (!expectAtom) break
          parenDepth++
          expectAtom = true
          i++
          continue
        }
        if (t.lit === '[') {
          bracketDepth++
          expectAtom = true
          i++
          continue
        }
        if (t.lit === '<') {
          angleDepth++
          expectAtom = true
          i++
          continue
        }
        if (t.lit === '|' || t.lit === '&' || t.lit === '.') {
          expectAtom = true
          i++
          continue
        }
        if (t.lit === 'as' && !expectAtom) {
          expectAtom = true
          i++
          continue
        }
        if (expectAtom && (t.kind === 'name' || t.lit === 'const' || t.lit === 'void' || t.lit === 'null')) {
          expectAtom = false
          i++
          continue
        }
        break
      }
      if (t.lit === '(') parenDepth++
      else if (t.lit === ')') {
        parenDepth = Math.max(0, parenDepth - 1)
        expectAtom = false
      } else if (t.lit === '{') braceDepth++
      else if (t.lit === '}') {
        braceDepth = Math.max(0, braceDepth - 1)
        expectAtom = false
      } else if (t.lit === '[') bracketDepth++
      else if (t.lit === ']') {
        bracketDepth = Math.max(0, bracketDepth - 1)
        expectAtom = false
      } else if (t.lit === '<') angleDepth++
      else if (t.lit === '>') {
        angleDepth = Math.max(0, angleDepth - 1)
        expectAtom = false
      }
      i++
    }
    return i - 1
  }

  private skipAngles(openIdx: number): number {
    let depth = 0
    for (let i = openIdx; i < this.tokens.length; i++) {
      if (this.tokens[i].lit === '<') depth++
      else if (this.tokens[i].lit === '>') {
        depth--
        if (depth === 0) return i + 1
      }
    }
    return openIdx
  }

  private collectParamNames(fromIdx: number, toIdx: number, kind: LocalKind, end: Position) {
    if (toIdx < 0) return
    const toks = this.tokens
    let expectingName = true
    let typeColonDepth = 0
    let parenDepth = 0
    let braceDepth = 0
    for (let i = fromIdx; i < toIdx; i++) {
      const t = toks[i]
      if (t.lit === '(') parenDepth++
      else if (t.lit === ')') parenDepth = Math.max(0, parenDepth - 1)
      else if (t.lit === '{') braceDepth++
      else if (t.lit === '}') braceDepth = Math.max(0, braceDepth - 1)

      if (t.lit === ':' && parenDepth === 0) {
        typeColonDepth++
        expectingName = false
        continue
      }
      if (t.lit === ',' && parenDepth === 0 && braceDepth === 0) {
        expectingName = true
        typeColonDepth = 0
        continue
      }
      if (t.lit === '=' && parenDepth === 0 && braceDepth === 0) {
        expectingName = false
        typeColonDepth = 0
        continue
      }
      if (typeColonDepth > 0) {
        if (t.kind === 'name') this.declTokenIndices.add(i)
        continue
      }
      if (expectingName && t.kind === 'name') {
        this.declTokenIndices.add(i)
        this.model.locals.push({
          name: t.lit,
          kind,
          start: { line: t.line, col: t.col },
          end,
          decl: { line: t.line, col: t.col },
        })
        expectingName = false
      }
    }
  }

  private collectVarBindings(startIdx: number, kind: LocalKind, depth: number, scopeEnd: Position) {
    const toks = this.tokens
    let i = startIdx
    let braceDepth = 0
    let bracketDepth = 0
    let expectingName = true
    let inType = false

    while (i < toks.length) {
      const t = toks[i]
      if (t.kind === 'eof') break
      if (braceDepth === 0 && bracketDepth === 0) {
        if (t.lit === ';' || t.lit === '=' || t.lit === ')' || (t.kind === 'name' && (t.lit === 'in' || t.lit === 'of'))) {
          break
        }
        if (t.kind === 'keyword') break
      }
      if (t.lit === '{') braceDepth++
      else if (t.lit === '}') braceDepth = Math.max(0, braceDepth - 1)
      else if (t.lit === '[') bracketDepth++
      else if (t.lit === ']') bracketDepth = Math.max(0, bracketDepth - 1)

      if (t.lit === ':' && braceDepth === 0 && bracketDepth === 0) {
        inType = true
        expectingName = false
        i++
        continue
      }
      if (t.lit === ',' && braceDepth === 0 && bracketDepth === 0) {
        inType = false
        expectingName = true
        i++
        continue
      }
      if (!inType && expectingName && t.kind === 'name') {
        this.declTokenIndices.add(i)
        if (depth === 0 && kind === 'local') {
          this.model.globals.add(t.lit)
          if (t.lit === 'loop') this.topLevelLoop = true
        }
        this.model.locals.push({
          name: t.lit,
          kind,
          start: { line: t.line, col: t.col },
          end: scopeEnd,
          decl: { line: t.line, col: t.col },
        })
        expectingName = braceDepth > 0 || bracketDepth > 0
      } else if (inType && t.kind === 'name') {
        this.declTokenIndices.add(i)
      }
      i++
    }
  }

  /** Second pass: validate prelude calls, capabilities, yields, return-from-loop, and globals. */
  analyzeRefs() {
    const toks = this.tokens
    const funcDepthStack: number[] = []
    let inLoopBody = false
    const braceInLoop: boolean[] = []

    for (let i = 0; i < toks.length; i++) {
      const t = toks[i]
      if (t.kind === 'eof') break

      if (t.lit === '{') {
        funcDepthStack.push(this.funcBraces.has(i) ? 1 : 0)
        const opensLoop = this.loopBodyBraces.has(i)
        braceInLoop.push(inLoopBody)
        if (opensLoop) inLoopBody = true
        continue
      }
      if (t.lit === '}') {
        funcDepthStack.pop()
        inLoopBody = braceInLoop.pop() ?? false
        continue
      }

      const funcDepth = funcDepthStack.reduce((sum, v) => sum + v, 0)

      if (t.kind === 'keyword' && t.lit === 'return') {
        // Warn when returning from the top-level loop() itself (funcDepth === 1 while inLoopBody).
        if (inLoopBody && funcDepth === 1) {
          this.diag(t, SeverityWarning, 'api.return-loop', i18n('script.diag.returnFromLoop'))
        }
        continue
      }

      if (t.kind !== 'name' || this.declTokenIndices.has(i)) continue

      // Property access after `.` or `?.`: handled from its root identifier.
      const prev = i > 0 ? toks[i - 1] : null
      if (prev && (prev.lit === '.' || prev.lit === '?.')) continue

      // Object literal key `{ foo: bar }` or TS annotation `: Type`.
      const next = toks[i + 1]
      if ((prev?.lit === '{' || prev?.lit === ',') && (next?.lit === ':' || next?.lit === '?:')) continue
      if (prev?.lit === ':' && this.typeNames.has(t.lit)) continue

      // TypeScript `expr as Type` / `expr as const`
      if (
        t.lit === 'as' &&
        prev &&
        prev.lit !== '.' &&
        prev.lit !== '?.' &&
        (prev.kind === 'name' ||
          prev.kind === 'string' ||
          prev.kind === 'number' ||
          prev.lit === ')' ||
          prev.lit === ']' ||
          prev.lit === '}') &&
        !this.localInScope('as', t.line, t.col) &&
        next &&
        (next.kind === 'name' || next.lit === '{' || next.lit === '[' || next.lit === '(' || next.lit === 'const' || next.lit === 'void' || next.lit === 'null')
      ) {
        i = this.skipTypeExprAfterAs(i + 1)
        continue
      }

      const rootName = t.lit
      const local = this.localInScope(rootName, t.line, t.col)

      const parts: Token[] = [t]
      let endIdx = i
      while (
        endIdx + 2 < toks.length &&
        (toks[endIdx + 1].lit === '.' || toks[endIdx + 1].lit === '?.') &&
        toks[endIdx + 2].kind === 'name'
      ) {
        parts.push(toks[endIdx + 2])
        endIdx += 2
      }
      const afterChain = toks[endIdx + 1]
      const isCall = afterChain?.lit === '('

      if (local) {
        i = endIdx
        continue
      }

      const unavailKey = unavailableGlobals.get(rootName)
      if (unavailKey && !this.model.globals.has(rootName)) {
        this.diag(t, SeverityWarning, 'sandbox.unavailable', i18n(unavailKey))
        i = endIdx
        continue
      }

      if (rootName === 'api') {
        this.checkExtAPICall(parts, funcDepth, isCall)
        i = endIdx
        continue
      }

      if (isGlobalName(rootName)) {
        this.checkPreludeChain(parts, funcDepth, isCall)
        i = endIdx
        continue
      }

      if (!isKnownGlobal(rootName) && !this.model.globals.has(rootName) && !this.typeNames.has(rootName)) {
        if (!this.hasImport) {
          const candidates = [...globalNames(), ...stdlibGlobals, ...this.model.globals, ...this.localNamesAt(t.line, t.col)]
          const s = suggest(rootName, candidates)
          this.diag(
            t,
            SeverityWarning,
            'global.unknown',
            s
              ? i18n('script.diag.unknownGlobalSuggest', { name: quote(rootName), suggestion: quote(s) })
              : i18n('script.diag.unknownGlobal', { name: quote(rootName) }),
          )
        }
      }
      i = endIdx
    }
  }

  private checkExtAPICall(parts: Token[], funcDepth: number, isCall: boolean) {
    const rootTok = parts[0]
    const fullPath = parts.map((p) => p.lit).join('.')
    if (parts.length === 1) return
    const knownEntry = parts.length === 2 ? lookup(fullPath) : undefined
    if (knownEntry) {
      if (knownEntry.capability && this.missingCapability(knownEntry.capability)) {
        this.diag(
          rootTok,
          SeverityWarning,
          'capability.missing',
          i18n('script.diag.capabilityMissing', { path: fullPath, capability: quote(knownEntry.capability) }),
          knownEntry.capability,
        )
      }
      if (knownEntry.yields && isCall && funcDepth === 0) {
        this.diag(rootTok, SeverityError, 'api.top-level-yield', i18n('script.diag.topLevelYield', { path: fullPath }))
      }
      return
    }
    if (parts.length >= 3) {
      if (this.missingCapability('apis')) {
        this.diag(
          rootTok,
          SeverityWarning,
          'capability.missing',
          i18n('script.diag.capabilityMissing', { path: fullPath, capability: quote('apis') }),
          'apis',
        )
      }
      if (isCall && funcDepth === 0) {
        this.diag(rootTok, SeverityError, 'api.top-level-yield', i18n('script.diag.topLevelYield', { path: fullPath }))
      }
    }
  }

  private checkPreludeChain(parts: Token[], funcDepth: number, isCall: boolean) {
    const rootTok = parts[0]
    let matchedPath = rootTok.lit
    let entry = lookup(matchedPath)

    for (let k = 1; k < parts.length; k++) {
      const nextPath = `${matchedPath}.${parts[k].lit}`
      const candidate = lookup(nextPath)
      if (!candidate) {
        if (entry?.kind === 'table') {
          const parentTok = parts[k]
          const s = suggest(parts[k].lit, fieldNames(matchedPath))
          const msg = s
            ? i18n('script.diag.unknownFieldSuggest', {
                field: quote(parts[k].lit),
                parent: quote(matchedPath),
                suggestion: quote(s),
              })
            : i18n('script.diag.unknownField', { field: quote(parts[k].lit), parent: quote(matchedPath) })
          this.diag(parentTok, SeverityWarning, 'field.unknown', msg)
          return
        }
        break
      }
      matchedPath = nextPath
      entry = candidate
    }

    if (!entry) return
    if (matchedPath === 'af.op') {
      this.diag(rootTok, SeverityInfo, 'api.af-op', i18n('script.diag.afOp'))
    }
    if (entry.maintenance) {
      this.diag(rootTok, SeverityWarning, 'api.maintenance-only', i18n('script.diag.maintenanceOnly', { path: matchedPath }))
    }
    if (entry.capability && this.missingCapability(entry.capability)) {
      this.diag(
        rootTok,
        SeverityWarning,
        'capability.missing',
        i18n('script.diag.capabilityMissing', { path: matchedPath, capability: quote(entry.capability) }),
        entry.capability,
      )
    }
    if (entry.yields && isCall && funcDepth === 0) {
      this.diag(rootTok, SeverityError, 'api.top-level-yield', i18n('script.diag.topLevelYield', { path: matchedPath }))
    }
  }

  private missingCapability(cap: string): boolean {
    if (!this.ctx.capabilities) return false
    return !this.ctx.capabilities.includes(cap)
  }

  private localInScope(name: string, line: number, col: number): Local | undefined {
    const pos = { line, col }
    for (let i = this.model.locals.length - 1; i >= 0; i--) {
      const l = this.model.locals[i]
      if (l.name !== name) continue
      if (pos.line < l.start.line || (pos.line === l.start.line && pos.col < l.start.col)) continue
      if (pos.line > l.end.line || (pos.line === l.end.line && pos.col > l.end.col)) continue
      return l
    }
    return undefined
  }

  private localNamesAt(line: number, col: number): string[] {
    const out: string[] = []
    for (const l of this.model.locals) {
      if (line < l.start.line || (line === l.start.line && col < l.start.col)) continue
      if (line > l.end.line || (line === l.end.line && col > l.end.col)) continue
      out.push(l.name)
    }
    return out
  }

  entryPointDiagnostics(): Diagnostic[] {
    if (this.topLevelLoop) return []
    if (this.nestedLoopTok) {
      return [
        {
          line: this.nestedLoopTok.line,
          col: this.nestedLoopTok.col,
          endLine: this.nestedLoopTok.endLine,
          endCol: this.nestedLoopTok.endCol,
          severity: SeverityWarning,
          code: 'entry.local-loop',
          message: i18n('script.diag.localLoop'),
        },
      ]
    }
    return [
      {
        line: this.eofPos.line,
        col: this.eofPos.col,
        endLine: this.eofPos.line,
        endCol: this.eofPos.col,
        severity: SeverityError,
        code: 'entry.missing-loop',
        message: i18n('script.diag.missingLoop'),
      },
    ]
  }
}
