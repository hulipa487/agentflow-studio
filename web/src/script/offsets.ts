// The offset-based face of the TypeScript/JS language service: what the
// builder and CodeMirror extension call. Everything here works in UTF-16 offsets
// and converts to the analyzer's line/column code-point positions at the edge.
import { currentLang, t as i18n } from '@/i18n'
import { analyzeSource, type AnalysisContext, type AnalysisResult, type Model } from './analysis'
import type { ApiEntry } from './api'
import { complete, completionScope, hoverAtPosition, type CompletionItem } from './service'
import type { Diagnostic, Position, SeverityLevel } from './token'

export type Severity = 'error' | 'warning' | 'info' | 'hint'

export interface ScriptDiagnostic {
  /** 0-based offsets into the source. */
  from: number
  to: number
  /** 1-based line of `from`, for lists outside the editor. */
  line: number
  severity: Severity
  message: string
  /** A stable rule id, e.g. "field.unknown" or "capability.missing". */
  code?: string
  /** Missing capability name when code === "capability.missing". */
  capability?: string
}

export interface ScriptOptions {
  /** The runtime's prelude version (catalog.prelude_version). */
  preludeVersion: number
  /**
   * The agent's effective capabilities, or null to skip capability checks.
   */
  capabilities: string[] | null
  /**
   * Whether the chunk must declare a top-level `loop` function. True by default;
   * false for support chunks, shared packages, and safety filters.
   */
  requireLoop?: boolean
}

const severityNames: Record<SeverityLevel, Severity> = { 1: 'error', 2: 'warning', 3: 'info', 4: 'hint' }

/** Maps the public options to the analyzer's context (defaulting prelude version to 2). */
export function contextFor(opts: ScriptOptions): AnalysisContext {
  return {
    preludeVersion: opts.preludeVersion > 0 ? opts.preludeVersion : 2,
    capabilities: opts.capabilities ?? null,
    requireLoop: opts.requireLoop ?? true,
  }
}

/** A cache key for a context; unknown capabilities and an empty list differ. */
export function contextKey(ctx: AnalysisContext): string {
  return `${ctx.preludeVersion}|${ctx.capabilities ? 'caps:' + ctx.capabilities.join('\u0000') : 'unknown'}|loop:${ctx.requireLoop !== false ? 1 : 0}`
}

const recent: { source: string; key: string; result: AnalysisResult }[] = []

/** Analyzes a chunk, reusing the result when the same text was just analyzed. */
export function analysisFor(source: string, ctx: AnalysisContext): AnalysisResult {
  const key = `${currentLang()}|${contextKey(ctx)}`
  for (let i = 0; i < recent.length; i++) {
    const hit = recent[i]
    if (hit.key === key && hit.source === source) {
      if (i > 0) {
        recent.splice(i, 1)
        recent.unshift(hit)
      }
      return hit.result
    }
  }
  const result = analyzeSource(source, ctx)
  recent.unshift({ source, key, result })
  if (recent.length > 4) recent.pop()
  return result
}

/** The UTF-16 offset of a line/column position, clamped to its line. */
export function offsetOf(model: Model, pos: Position): number {
  const text = model.text
  if (pos.line >= text.starts.length) return text.text.length
  const col = Math.max(0, Math.min(pos.col, text.lineLength(pos.line)))
  return text.toUtf16(text.starts[pos.line] + col)
}

/** The line/column position of a UTF-16 offset. */
export function positionOf(model: Model, offset: number): Position {
  return model.text.position(model.text.fromUtf16(offset))
}

/** Converts analyzer diagnostics to offset ranges. */
export function toScriptDiagnostics(result: AnalysisResult): ScriptDiagnostic[] {
  return result.diagnostics.map((d) => toScriptDiagnostic(result.model, d))
}

function toScriptDiagnostic(model: Model, d: Diagnostic): ScriptDiagnostic {
  const from = offsetOf(model, { line: d.line, col: d.col })
  const to = Math.max(from, offsetOf(model, { line: d.endLine, col: d.endCol }))
  return {
    from,
    to,
    line: d.line + 1,
    severity: severityNames[d.severity] ?? 'warning',
    message: d.message,
    code: d.code,
    ...(d.capability ? { capability: d.capability } : {}),
  }
}

/** Analyzes a loop chunk and returns its diagnostics. Pure; safe to call on every keystroke. */
export function analyze(source: string, opts: ScriptOptions): ScriptDiagnostic[] {
  return toScriptDiagnostics(analysisFor(source, contextFor(opts)))
}

export interface ScriptCompletions {
  /** Where the typed prefix starts; accepting an item replaces from..to. */
  from: number
  to: number
  /** The identifier already typed before the cursor. */
  prefix: string
  /** The dotted path before the prefix ("session.state"), or "" at the root. */
  path: string
  /** Matching items, sorted by label. */
  items: CompletionItem[]
}

/** Completion at an offset. */
export function completionsAt(source: string, offset: number, opts: ScriptOptions): ScriptCompletions {
  return completionsIn(analysisFor(source, contextFor(opts)).model, offset)
}

export function completionsIn(model: Model, offset: number): ScriptCompletions {
  const pos = positionOf(model, offset)
  const scope = completionScope(model, pos.line, pos.col)
  return {
    from: model.text.toUtf16(scope.start),
    to: model.text.toUtf16(scope.end),
    prefix: scope.prefix,
    path: scope.path,
    items: complete(model, pos.line, pos.col),
  }
}

export interface ScriptHover {
  /** The hovered symbol range: the whole dotted path. */
  from: number
  to: number
  /** The card as Markdown. */
  markdown: string
  /** The prelude entry, when the path is documented. */
  entry?: ApiEntry
}

/** Hover documentation for the symbol at an offset, or null. */
export function hoverAt(source: string, offset: number, opts: ScriptOptions): ScriptHover | null {
  return hoverIn(analysisFor(source, contextFor(opts)).model, offset)
}

export function hoverIn(model: Model, offset: number): ScriptHover | null {
  const pos = positionOf(model, offset)
  const h = hoverAtPosition(model, pos.line, pos.col)
  if (!h) return null
  const from = offsetOf(model, { line: h.line, col: h.col })
  const to = Math.max(from, offsetOf(model, { line: h.endLine, col: h.endCol }))
  return { from, to, markdown: h.markdown, entry: h.entry }
}

export interface DiagnosticSummary {
  errors: number
  warnings: number
  text: string
  tone: 'success' | 'warning' | 'danger'
}

/** The loop editor's status line for a set of diagnostics. */
export function summarize(diagnostics: readonly { severity: Severity }[]): DiagnosticSummary {
  let errors = 0
  let warnings = 0
  for (const d of diagnostics) {
    if (d.severity === 'error') errors++
    else if (d.severity === 'warning') warnings++
  }
  if (errors > 0) {
    const text = warnings > 0 ? i18n('script.status.errorsAndWarnings', { errors, warnings }) : i18n('script.status.errors', { errors })
    return { errors, warnings, text, tone: 'danger' }
  }
  if (warnings > 0) return { errors, warnings, text: i18n('script.status.warnings', { warnings }), tone: 'warning' }
  return { errors, warnings, text: i18n('script.status.noIssues'), tone: 'success' }
}
