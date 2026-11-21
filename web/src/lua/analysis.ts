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
   * The runtime's prelude version. Zero disables the version comparison (but
   * not the directive requirement).
   */
  preludeVersion: number
  /**
   * The agent's effective capability list. null (or omitted) means "unknown,
   * do not check"; an empty list means no capabilities.
   */
  capabilities?: readonly string[] | null
}

export type LocalKind = 'local' | 'param' | 'loop' | 'self' | 'function'

/** A name declared inside the chunk, with the scope it lives in. */
export interface Local {
  name: string
  kind: LocalKind
  start: Position
  /** {0, 0} until the scope closes. */
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

/** Lints one loop source. */
export function analyzeSource(src: string, ctx: AnalysisContext): AnalysisResult {
  const text = new RuneText(src)
  const { tokens, diagnostics: lexDiags, comments } = lex(text)

  let diags = lexDiags.slice()
  diags.push(...checkDirective(src, ctx.preludeVersion))
  diags.push(...checkStructure(tokens))

  const a = new Analyzer(src, tokens, ctx, {
    source: src,
    text,
    tokens,
    comments,
    locals: [],
    globals: new Set(),
  })
  a.run()
  diags.push(...a.diags)
  diags.push(...a.entryPointDiagnostics())

  diags = dedupeDiagnostics(diags)
  // Array.prototype.sort is stable, as the Go original requires.
  diags.sort((x, y) => {
    if (x.line !== y.line) return x.line - y.line
    if (x.col !== y.col) return x.col - y.col
    return x.severity - y.severity
  })
  return { diagnostics: diags, model: a.model }
}

function dedupeDiagnostics(input: Diagnostic[]): Diagnostic[] {
  const seen = new Set<string>()
  const out: Diagnostic[] = []
  for (const d of input) {
    const key = `${d.code}|${d.line}|${d.col}|${d.message}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push(d)
  }
  return out
}

// ---- directive -------------------------------------------------------------

// Go's \s is ASCII-only; spell it out so both analyzers accept the same lines.
const versionDirectiveRE = /^[\t\n\f\r ]*--[\t\n\f\r ]*af-prelude-version[\t\n\f\r ]*:[\t\n\f\r ]*(\d+)[\t\n\f\r ]*$/

/**
 * Mirrors the runtime's gate: the directive must appear exactly once in the
 * leading run of blank and comment lines.
 */
function checkDirective(src: string, want: number): Diagnostic[] {
  const hits: { version: number; line: number }[] = []
  const lines = src.split('\n')
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const t = trimSpace(line)
    if (t === '') continue
    if (!t.startsWith('--')) break
    const m = versionDirectiveRE.exec(line)
    if (m) hits.push({ version: Number(m[1]), line: i })
  }
  if (hits.length === 0) {
    return [
      {
        line: 0,
        col: 0,
        endLine: 0,
        endCol: 1,
        severity: SeverityError,
        code: 'prelude.missing-directive',
        message: i18n('lua.diag.missingDirective', { want }),
      },
    ]
  }
  const out: Diagnostic[] = []
  for (const h of hits.slice(1)) {
    out.push({
      line: h.line,
      col: 0,
      endLine: h.line,
      endCol: 1,
      severity: SeverityError,
      code: 'prelude.duplicate-directive',
      message: i18n('lua.diag.duplicateDirective'),
    })
  }
  if (want > 0) {
    for (const h of hits) {
      if (h.version !== want) {
        out.push({
          line: h.line,
          col: 0,
          endLine: h.line,
          endCol: 1,
          severity: SeverityError,
          code: 'prelude.version-mismatch',
          message: i18n('lua.diag.versionMismatch', { got: h.version, want }),
        })
      }
    }
  }
  return out
}

// ---- structure -------------------------------------------------------------

interface BlockFrame {
  kind: string
  tok: Token
}

const closers: Record<string, string> = { ')': '(', ']': '[', '}': '{' }
const luauOps = new Set(['+=', '-=', '*=', '/=', '%=', '^=', '..=', '//=', '//'])

function checkStructure(tokens: Token[]): Diagnostic[] {
  const diags: Diagnostic[] = []
  const blocks: BlockFrame[] = []
  const brackets: { lit: string; tok: Token }[] = []
  let pendingIf = 0
  let pendingLoop = ''

  const report = (t: Token, code: string, message: string) => {
    diags.push({
      line: t.line,
      col: t.col,
      endLine: t.endLine,
      endCol: t.endCol,
      severity: SeverityError,
      code,
      message,
    })
  }

  for (const t of tokens) {
    if (t.kind === 'eof') break
    if (t.kind === 'op') {
      if (luauOps.has(t.lit)) {
        report(t, 'syntax.luau-only', i18n('lua.diag.luauOperator', { op: quote(t.lit) }))
      }
      switch (t.lit) {
        case '(':
        case '[':
        case '{':
          brackets.push({ lit: t.lit, tok: t })
          break
        case ')':
        case ']':
        case '}': {
          const want = closers[t.lit]
          if (brackets.length === 0) {
            report(t, 'syntax.unexpected-close', i18n('lua.diag.unexpectedClose', { token: quote(t.lit) }))
          } else {
            const top = brackets[brackets.length - 1]
            if (top.lit !== want) {
              report(t, 'syntax.mismatched-close', i18n('lua.diag.mismatchedClose', { token: quote(t.lit), open: quote(top.lit) }))
            }
            brackets.pop()
          }
          break
        }
      }
    }
    if (t.kind !== 'keyword') continue
    switch (t.lit) {
      case 'continue':
        report(t, 'syntax.luau-only', i18n('lua.diag.luauContinue'))
        break
      case 'function':
        blocks.push({ kind: 'function', tok: t })
        break
      case 'if':
        pendingIf++
        break
      case 'then':
        if (pendingIf > 0) {
          blocks.push({ kind: 'if', tok: t })
          pendingIf--
        }
        break
      case 'for':
        pendingLoop = 'for'
        break
      case 'while':
        pendingLoop = 'while'
        break
      case 'do':
        if (pendingLoop !== '') {
          blocks.push({ kind: pendingLoop, tok: t })
          pendingLoop = ''
        } else {
          blocks.push({ kind: 'do', tok: t })
        }
        break
      case 'repeat':
        blocks.push({ kind: 'repeat', tok: t })
        break
      case 'elseif':
      case 'else':
        if (blocks.length === 0 || blocks[blocks.length - 1].kind !== 'if') {
          report(t, 'syntax.unexpected-else', i18n('lua.diag.unexpectedElse', { token: quote(t.lit) }))
        }
        break
      case 'until':
        if (blocks.length === 0 || blocks[blocks.length - 1].kind !== 'repeat') {
          report(t, 'syntax.unexpected-until', i18n('lua.diag.unexpectedUntil'))
        } else {
          blocks.pop()
        }
        break
      case 'end': {
        if (blocks.length === 0) {
          report(t, 'syntax.unexpected-end', i18n('lua.diag.unexpectedEnd'))
          break
        }
        const top = blocks[blocks.length - 1]
        if (top.kind === 'repeat') {
          report(t, 'syntax.mismatched-end', i18n('lua.diag.mismatchedEnd'))
        }
        blocks.pop()
        break
      }
    }
  }
  for (const b of blocks) {
    diags.push({
      line: b.tok.line,
      col: b.tok.col,
      endLine: b.tok.endLine,
      endCol: b.tok.endCol,
      severity: SeverityError,
      code: 'syntax.unclosed-block',
      message: i18n('lua.diag.unclosedBlock', { kind: b.kind }),
    })
  }
  for (const b of brackets) {
    diags.push({
      line: b.tok.line,
      col: b.tok.col,
      endLine: b.tok.endLine,
      endCol: b.tok.endCol,
      severity: SeverityError,
      code: 'syntax.unclosed-bracket',
      message: i18n('lua.diag.unclosedBracket', { token: quote(b.lit) }),
    })
  }
  return diags
}

// ---- scope and API analysis ------------------------------------------------

interface ScopeFrame {
  kind: 'chunk' | 'block' | 'function' | 'loop'
  vars: Map<string, Local>
  fn: boolean
  /** The body of the global function loop(). */
  loop: boolean
  start: Position
}

function posOf(t: Token): Position {
  return { line: t.line, col: t.col }
}

function frame(kind: ScopeFrame['kind'], start: Position, fn = false): ScopeFrame {
  return { kind, vars: new Map(), fn, loop: false, start }
}

function newLocal(name: string, kind: LocalKind, at: Position): Local {
  return { name, kind, start: at, end: { line: 0, col: 0 }, decl: at }
}

function is(t: Token | undefined, lit: string): boolean {
  return t !== undefined && t.lit === lit
}

class Analyzer {
  readonly model: Model
  readonly diags: Diagnostic[] = []
  private readonly src: string
  private readonly tokens: Token[]
  private readonly ctx: AnalysisContext
  private readonly skip = new Set<number>()
  private frames: ScopeFrame[] = []

  private tableDepth = 0
  /** Identifiers on this line are type syntax. */
  private typeLine = -1
  private loopLocal = false

  private pendingLoopNames: string[] | null = null
  private pendingIf = 0
  private readonly hasImports: boolean

  constructor(src: string, tokens: Token[], ctx: AnalysisContext, model: Model) {
    this.src = src
    this.tokens = tokens
    this.ctx = ctx
    this.model = model
    this.hasImports = hasPackageImports(src)
  }

  run() {
    const tokens = this.tokens
    this.frames = [frame('chunk', { line: 0, col: 0 })]
    this.typeLine = -1

    for (let i = 0; i < tokens.length; i++) {
      const t = tokens[i]
      if (t.kind === 'eof') break
      if (this.typeLine >= 0 && t.line !== this.typeLine) this.typeLine = -1
      if (this.typeLine >= 0 && t.kind === 'name') continue
      if (this.skip.has(i)) continue
      if (t.lit === '{') {
        this.tableDepth++
      } else if (t.lit === '}') {
        if (this.tableDepth > 0) this.tableDepth--
      }
      if (t.kind === 'keyword') {
        switch (t.lit) {
          case 'function': {
            const [f, next] = this.handleFunction(i, false)
            this.frames.push(f)
            i = next - 1
            break
          }
          case 'local':
            i = this.handleLocal(i)
            break
          case 'for':
            this.pendingLoopNames = this.parseForHeader(i)
            break
          case 'if':
            // The body starts at the first then; an elseif then continues the
            // same block rather than opening another one.
            this.pendingIf++
            break
          case 'then':
            if (this.pendingIf > 0) {
              this.pendingIf--
              this.frames.push(frame('block', posOf(t)))
            }
            break
          case 'do': {
            const f = frame('block', posOf(t))
            if (this.pendingLoopNames !== null) {
              f.kind = 'loop'
              for (const name of this.pendingLoopNames) {
                const l = newLocal(name, 'loop', posOf(t))
                f.vars.set(name, l)
                this.model.locals.push(l)
              }
              this.pendingLoopNames = null
            }
            this.frames.push(f)
            break
          }
          case 'repeat':
            this.frames.push(frame('block', posOf(t)))
            break
          case 'until':
            this.pop(posOf(t))
            if (i + 1 < tokens.length) this.typeLine = -1
            break
          case 'end':
            this.pop(posOf(t))
            break
          case 'return': {
            const fn = this.nearestFunction()
            if (fn !== null && fn.loop) {
              this.diag(
                t,
                SeverityWarning,
                'api.return-loop',
                i18n('lua.diag.returnFromLoop'),
              )
            }
            break
          }
        }
        continue
      }
      if (t.kind === 'name') {
        const prev = i > 0 ? tokens[i - 1] : undefined
        const atStmtStart =
          prev === undefined ||
          prev.line !== t.line ||
          prev.lit === ';' ||
          (prev.kind === 'keyword' && (prev.lit === 'then' || prev.lit === 'do' || prev.lit === 'else' || prev.lit === 'repeat'))
        if (atStmtStart && t.lit === 'export' && tokens[i + 1]?.kind === 'name' && tokens[i + 1].lit === 'type' && tokens[i + 2]?.kind === 'name') {
          this.diag(t, SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
          this.typeLine = t.line
          continue
        }
        if (
          atStmtStart &&
          t.lit === 'type' &&
          tokens[i + 1]?.kind === 'name' &&
          (is(tokens[i + 2], '=') || is(tokens[i + 2], '<'))
        ) {
          this.diag(t, SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
          this.typeLine = t.line
          continue
        }
        i = this.handleName(i)
      }
    }
    this.finish()
  }

  /**
   * Closes every open scope, then the chunk, so locals have an end position
   * for completion.
   */
  private finish() {
    const end = posOf(this.tokens[this.tokens.length - 1])
    while (this.frames.length > 1) this.pop(end)
    for (const l of this.frames[0].vars.values()) l.end = end
  }

  private pop(at: Position) {
    if (this.frames.length <= 1) return
    const f = this.frames.pop() as ScopeFrame
    for (const l of f.vars.values()) l.end = at
  }

  private nearestFunction(): ScopeFrame | null {
    for (let i = this.frames.length - 1; i >= 0; i--) {
      if (this.frames[i].fn) return this.frames[i]
    }
    return null
  }

  private declare(t: Token, kind: LocalKind): Local {
    const l = newLocal(t.lit, kind, posOf(t))
    this.frames[this.frames.length - 1].vars.set(t.lit, l)
    this.model.locals.push(l)
    return l
  }

  private resolveLocal(name: string): boolean {
    for (let i = this.frames.length - 1; i >= 0; i--) {
      if (this.frames[i].vars.has(name)) return true
    }
    return false
  }

  private diag(t: Token, severity: SeverityLevel, code: string, message: string, capability?: string) {
    this.diags.push({
      line: t.line,
      col: t.col,
      endLine: t.endLine,
      endCol: t.endCol,
      severity,
      code,
      message,
      ...(capability ? { capability } : {}),
    })
  }

  /**
   * Parses a function header, declares its name and parameters, and returns
   * the frame for its body plus the token index after the header.
   */
  private handleFunction(at: number, local: boolean): [ScopeFrame, number] {
    const tokens = this.tokens
    const t = tokens[at]
    let j = at + 1
    let name = ''
    let simple = false
    let dotted = false
    let method = false
    let rootTok: Token | null = null

    if (j < tokens.length && tokens[j].kind === 'name') {
      name = tokens[j].lit
      rootTok = tokens[j]
      j++
      while (
        j + 1 < tokens.length &&
        (tokens[j].lit === '.' || tokens[j].lit === ':') &&
        tokens[j + 1].kind === 'name'
      ) {
        if (tokens[j].lit === ':') method = true
        dotted = true
        j += 2
      }
      if (is(tokens[j], '<')) {
        this.diag(tokens[j], SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
        j = this.skipAngles(j)
      }
      simple = !dotted && is(tokens[j], '(')
    }

    const f = frame('function', posOf(t), true)
    if (rootTok !== null) {
      if (simple) {
        if (local) {
          this.declare(rootTok, 'function')
          if (name === 'loop') this.loopLocal = true
        } else {
          // Lua function statements create globals.
          this.model.globals.add(name)
          if (name === 'loop') f.loop = true
        }
      } else if (dotted) {
        // function a.b() assigns a field; a must resolve.
        this.checkRootUse(rootTok)
      }
    }
    if (method) {
      const l = newLocal('self', 'self', posOf(t))
      f.vars.set('self', l)
      this.model.locals.push(l)
    }
    if (is(tokens[j], '(')) j = this.parseParams(j, f)
    if (is(tokens[j], ':')) {
      this.diag(tokens[j], SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
      // Return type: identifiers on this line are types, not globals.
      this.typeLine = tokens[j].line
      j++
    }
    return [f, j]
  }

  private checkRootUse(t: Token) {
    if (this.resolveLocal(t.lit)) return
    if (isKnownGlobal(t.lit) || this.model.globals.has(t.lit)) return
    const unavailable = unavailableGlobals.get(t.lit)
    if (unavailable !== undefined) {
      this.diag(t, SeverityError, 'sandbox.unavailable', i18n(unavailable))
      return
    }
    if (this.typeLine >= 0) return
    const suggestion = suggest(t.lit, this.knownNames())
    const msg =
      suggestion !== ''
        ? i18n('lua.diag.unknownGlobalSuggest', { name: quote(t.lit), suggestion: quote(suggestion) })
        : i18n('lua.diag.unknownGlobal', { name: quote(t.lit) })
    this.diag(t, SeverityWarning, 'global.unknown', msg)
  }

  private parseParams(open: number, f: ScopeFrame): number {
    const tokens = this.tokens
    let depth = 0
    let expect = true
    for (let j = open; j < tokens.length; j++) {
      const t = tokens[j]
      if (t.lit === '(') {
        depth++
        continue
      }
      if (t.lit === ')') {
        depth--
        if (depth === 0) return j + 1
        continue
      }
      if (depth !== 1) continue
      if (t.lit === ',') {
        expect = true
        continue
      }
      if (t.lit === ':') {
        this.diag(t, SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
        j = this.skipTypeUntil(j + 1, ',', ')') - 1
        continue
      }
      if (expect && t.kind === 'name') {
        const l = newLocal(t.lit, 'param', posOf(t))
        f.vars.set(t.lit, l)
        this.model.locals.push(l)
        expect = false
      }
    }
    return tokens.length
  }

  /**
   * Advances over a type expression, stopping before stop tokens at bracket
   * depth zero. It returns the index of the stop token.
   */
  private skipTypeUntil(from: number, ...stops: string[]): number {
    const tokens = this.tokens
    let depth = 0
    for (let j = from; j < tokens.length; j++) {
      const t = tokens[j]
      if (t.kind === 'eof') return j
      switch (t.lit) {
        case '(':
        case '{':
        case '[':
        case '<':
          depth++
          continue
        case ')':
        case '}':
        case ']':
        case '>':
          if (depth === 0) return j
          depth--
          continue
      }
      if (depth === 0 && stops.includes(t.lit)) return j
    }
    return tokens.length
  }

  private skipAngles(at: number): number {
    const tokens = this.tokens
    let depth = 0
    for (let j = at; j < tokens.length; j++) {
      const lit = tokens[j].lit
      if (lit === '<') {
        depth++
      } else if (lit === '>') {
        depth--
        if (depth === 0) return j + 1
      }
    }
    return at + 1
  }

  private handleLocal(at: number): number {
    const tokens = this.tokens
    if (is(tokens[at + 1], 'function')) {
      const [f, next] = this.handleFunction(at + 1, true)
      this.frames.push(f)
      return next - 1
    }

    let j = at + 1
    let expect = true
    while (j < tokens.length) {
      const t = tokens[j]
      if (t.kind === 'eof') break
      if (t.lit === ',') {
        expect = true
        j++
        continue
      }
      if (t.lit === '=') break
      if (t.lit === ':') {
        this.diag(t, SeverityError, 'syntax.luau-only', i18n('lua.diag.luauTypes'))
        j = this.skipTypeUntil(j + 1, ',', '=')
        expect = false
        continue
      }
      if (expect && t.kind === 'name') {
        this.declare(t, 'local')
        expect = false
        j++
        continue
      }
      break
    }
    return j - 1
  }

  /**
   * Collects the loop variable names of a for header; null when it has none,
   * so the following do opens a plain block.
   */
  private parseForHeader(at: number): string[] | null {
    const tokens = this.tokens
    const names: string[] = []
    let expect = true
    let depth = 0
    let mode = 'names'
    let doAt = -1
    for (let j = at + 1; j < tokens.length; j++) {
      const t = tokens[j]
      if (t.kind === 'eof') break
      if (t.lit === '(' || t.lit === '{' || t.lit === '[') {
        depth++
        continue
      }
      if (t.lit === ')' || t.lit === '}' || t.lit === ']') {
        if (depth > 0) depth--
        continue
      }
      if (depth !== 0) continue
      if (t.lit === 'do') {
        doAt = j
        break
      }
      if (t.lit === ',' && mode === 'names') {
        expect = true
        continue
      }
      if (t.lit === '=' || t.lit === 'in') {
        mode = 'expr'
        continue
      }
      if (mode === 'names' && expect && t.kind === 'name') {
        names.push(t.lit)
        this.skip.add(j)
        expect = false
      }
    }
    if (doAt >= 0) {
      // A generic for over llm.stream is refused by the bridge.
      for (let j = at + 1; j < doAt; j++) {
        if (tokens[j].lit === 'llm' && j + 2 < tokens.length && tokens[j + 1].lit === '.' && tokens[j + 2].lit === 'stream') {
          this.diag(
            tokens[j],
            SeverityError,
            'api.stream-for',
            i18n('lua.diag.streamFor'),
          )
          break
        }
      }
    }
    return names.length > 0 ? names : null
  }

  private handleName(at: number): number {
    const tokens = this.tokens
    const t = tokens[at]
    if (at > 0 && (tokens[at - 1].lit === '.' || tokens[at - 1].lit === ':')) return at
    // Table constructor key: { name = ... }
    if (
      this.tableDepth > 0 &&
      is(tokens[at + 1], '=') &&
      (at === 0 || tokens[at - 1].lit === '{' || tokens[at - 1].lit === ',' || tokens[at - 1].lit === ';')
    ) {
      return at
    }

    // Build the dotted path so field names are not treated as globals.
    const parts = [t.lit]
    let j = at
    while (
      j + 2 < tokens.length &&
      (tokens[j + 1].lit === '.' || tokens[j + 1].lit === ':') &&
      tokens[j + 2].kind === 'name'
    ) {
      parts.push(tokens[j + 2].lit)
      j += 2
    }

    if (this.resolveLocal(t.lit)) return j

    const declared = isKnownGlobal(t.lit) || this.model.globals.has(t.lit)
    const unavailable = unavailableGlobals.get(t.lit)
    if (unavailable !== undefined && !declared) {
      this.diag(t, SeverityError, 'sandbox.unavailable', i18n(unavailable))
      return j
    }

    if (!declared) {
      if (this.typeLine >= 0) return j
      if (is(tokens[at + 1], '=')) {
        const suggestion = suggest(t.lit, this.knownNames())
        const msg =
          suggestion !== ''
            ? i18n('lua.diag.undeclaredAssignmentSuggest', { name: quote(t.lit), suggestion: quote(suggestion) })
            : i18n('lua.diag.undeclaredAssignment', { name: quote(t.lit) })
        this.diag(t, SeverityWarning, 'global.assignment', msg)
        return j
      }
      if (this.hasImports) {
        // A package import contributes source the analyzer cannot see; its
        // globals would otherwise all be reported as unknown.
        return j
      }
      const suggestion = suggest(t.lit, this.knownNames())
      const msg =
        suggestion !== ''
          ? i18n('lua.diag.unknownGlobalSuggest', { name: quote(t.lit), suggestion: quote(suggestion) })
          : i18n('lua.diag.unknownGlobal', { name: quote(t.lit) })
      this.diag(t, SeverityWarning, 'global.unknown', msg)
      return j
    }

    // Validate the field chain against the documented prelude. Standard
    // library tables (string, table, math, ...) are deliberately not
    // enumerated: the analyzer only knows the AgentFlow surface.
    if (parts.length > 1 && isGlobalName(parts[0])) {
      for (let k = 1; k < parts.length; k++) {
        const prefix = parts.slice(0, k + 1).join('.')
        if (lookup(prefix) !== undefined) continue
        const parent = parts.slice(0, k).join('.')
        const fields = fieldNames(parent)
        const segTok = tokens[at + 2 * k]
        let msg = i18n('lua.diag.unknownField', { field: quote(parts[k]), parent: quote(parent) })
        const s = suggest(parts[k], fields)
        if (s !== '') {
          msg = i18n('lua.diag.unknownFieldSuggest', { field: quote(parts[k]), parent: quote(parent), suggestion: quote(s) })
        } else if (fields.length === 0) {
          msg = i18n('lua.diag.noPublicFields', { parent: quote(parent) })
        }
        this.diag(segTok, SeverityWarning, 'field.unknown', msg)
        return j
      }
    }

    const path = parts.join('.')
    const entry = lookup(path)
    if (entry === undefined) return j
    if (path === 'af.op') {
      this.diag(
        t,
        SeverityWarning,
        'api.af-op',
        i18n('lua.diag.afOp'),
      )
    }
    // A call is a path immediately followed by "(".
    if (!is(tokens[j + 1], '(')) return j
    if (entry.maintenance) {
      this.diag(t, SeverityWarning, 'api.maintenance-only', i18n('lua.diag.maintenanceOnly', { path }))
    }
    const caps = this.ctx.capabilities
    if (entry.capability && caps !== null && caps !== undefined) {
      if (!caps.includes(entry.capability)) {
        this.diag(
          t,
          SeverityWarning,
          'capability.missing',
          i18n('lua.diag.capabilityMissing', { path, capability: quote(entry.capability) }),
          entry.capability,
        )
      }
    }
    if (entry.yields && this.nearestFunction() === null) {
      this.diag(
        t,
        SeverityWarning,
        'api.top-level-yield',
        i18n('lua.diag.topLevelYield', { path }),
      )
    }
    return j
  }

  // Go ranges over maps here, so which of several equally close names wins is
  // unspecified there; this order (prelude, stdlib, chunk globals, locals) is
  // one of its outcomes, made deterministic.
  private knownNames(): string[] {
    const seen = new Set<string>()
    const add = (n: string) => {
      if (n !== '') seen.add(n)
    }
    globalNames().forEach(add)
    stdlibGlobals.forEach(add)
    this.model.globals.forEach(add)
    for (const l of this.model.locals) add(l.name)
    return [...seen]
  }

  entryPointDiagnostics(): Diagnostic[] {
    if (this.model.globals.has('loop')) return []
    if (this.loopLocal) {
      return [
        {
          line: 0,
          col: 0,
          endLine: 0,
          endCol: 1,
          severity: SeverityError,
          code: 'entry.local-loop',
          message: i18n('lua.diag.localLoop'),
        },
      ]
    }
    const line = this.src.split('\n').length - 1
    return [
      {
        line,
        col: 0,
        endLine: line,
        endCol: 1,
        severity: SeverityError,
        code: 'entry.missing-loop',
        message: i18n('lua.diag.missingLoop'),
      },
    ]
  }
}

/**
 * Reports whether the chunk imports a deployment package. Package exports are
 * concatenated before the chunk at boot, so names they define are invisible to
 * this analyzer; unknown-global warnings are skipped for such chunks rather
 * than reporting every package call as a typo.
 */
export function hasPackageImports(src: string): boolean {
  for (const line of src.split('\n')) {
    const t = trimSpace(line)
    if (t === '' || t.startsWith('--')) {
      const body = t.startsWith('--') ? t.slice(2) : t
      if (trimSpace(body).startsWith('af-import:')) return true
      continue
    }
    break
  }
  return false
}
