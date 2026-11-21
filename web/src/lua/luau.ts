// The offset-based face of the language service: what the builder and the
// CodeMirror extension call. Everything here works in UTF-16 offsets (plain
// JavaScript string indexes) and converts to the analyzer's line/column
// code-point positions at the edge.
import { currentLang, t as i18n } from '@/i18n'
import { analyzeSource, type AnalysisContext, type AnalysisResult, type Model } from './analysis'
import type { ApiEntry } from './api'
import { complete, completionScope, hoverAtPosition, type CompletionItem } from './service'
import type { Diagnostic, Position, SeverityLevel } from './token'

export type Severity = 'error' | 'warning' | 'info' | 'hint'

export interface LuauDiagnostic {
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

export interface LuauOptions {
  /** The runtime's prelude version (catalog.prelude_version). */
  preludeVersion: number
  /**
   * The agent's effective capabilities: the selected list, or the deployment
   * defaults when nothing is selected. null means they are unknown and skips
   * the capability checks entirely; an empty list means the agent has none, so
   * every capability-gated call is flagged. The Go analyzer draws the same
   * line between a nil and an empty list.
   */
  capabilities: string[] | null
}

const severityNames: Record<SeverityLevel, Severity> = { 1: 'error', 2: 'warning', 3: 'info', 4: 'hint' }

/**
 * Maps the public options to the analyzer's context. Like the desktop loop
 * editor, a missing prelude version counts as 1.
 */
export function contextFor(opts: LuauOptions): AnalysisContext {
  return {
    preludeVersion: opts.preludeVersion > 0 ? opts.preludeVersion : 1,
    capabilities: opts.capabilities ?? null,
  }
}

/** A cache key for a context; unknown capabilities and an empty list differ. */
export function contextKey(ctx: AnalysisContext): string {
  return `${ctx.preludeVersion}|${ctx.capabilities ? 'caps:' + ctx.capabilities.join('\u0000') : 'unknown'}`
}

// The linter, the completion source, the hover source and the builder's own
// diagnostics list all ask about the same text within one keystroke; a few
// remembered results turn those into one analysis. Diagnostics are worded in
// the language current at analysis time, so the language is part of the key.
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
export function toLuauDiagnostics(result: AnalysisResult): LuauDiagnostic[] {
  return result.diagnostics.map((d) => toLuauDiagnostic(result.model, d))
}

function toLuauDiagnostic(model: Model, d: Diagnostic): LuauDiagnostic {
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
export function analyze(source: string, opts: LuauOptions): LuauDiagnostic[] {
  return toLuauDiagnostics(analysisFor(source, contextFor(opts)))
}

export interface LuauCompletions {
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

/**
 * Completion at an offset: after `name.` or `name:` the fields of that prelude
 * namespace, otherwise locals in scope, prelude globals, the standard library
 * and the chunk's own functions, all filtered by the typed prefix.
 */
export function completionsAt(source: string, offset: number, opts: LuauOptions): LuauCompletions {
  return completionsIn(analysisFor(source, contextFor(opts)).model, offset)
}

export function completionsIn(model: Model, offset: number): LuauCompletions {
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

export interface LuauHover {
  /** The hovered symbol's range: the whole dotted path. */
  from: number
  to: number
  /** The card as Markdown: a bold signature line, then paragraphs. */
  markdown: string
  /** The prelude entry, when the path is a documented one. */
  entry?: ApiEntry
}

/** Hover documentation for the symbol at an offset, or null. */
export function hoverAt(source: string, offset: number, opts: LuauOptions): LuauHover | null {
  return hoverIn(analysisFor(source, contextFor(opts)).model, offset)
}

export function hoverIn(model: Model, offset: number): LuauHover | null {
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
  /** "no issues", "1 warning", "2 errors · 1 warning": the desktop's status line. */
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
    const text = warnings > 0 ? i18n('lua.status.errorsAndWarnings', { errors, warnings }) : i18n('lua.status.errors', { errors })
    return { errors, warnings, text, tone: 'danger' }
  }
  if (warnings > 0) return { errors, warnings, text: i18n('lua.status.warnings', { warnings }), tone: 'warning' }
  return { errors, warnings, text: i18n('lua.status.noIssues'), tone: 'success' }
}
