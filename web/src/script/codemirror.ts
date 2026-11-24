// The CodeMirror face of the TypeScript/JS Prelude v2 language service:
// diagnostics, completion, and hover wired the way the loop editor presents them.
import {
  autocompletion,
  completionStatus,
  selectedCompletion,
  type Completion,
  type CompletionContext,
  type CompletionResult,
} from '@codemirror/autocomplete'
import { forEachDiagnostic, lintGutter, linter, type Diagnostic as CmDiagnostic } from '@codemirror/lint'
import { Prec, type EditorState, type Extension, type Text } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  ViewPlugin,
  WidgetType,
  hoverTooltip,
  type DecorationSet,
  type Tooltip,
  type ViewUpdate,
} from '@codemirror/view'
import { currentLang, t as i18n, type Lang } from '@/i18n'
import type { AnalysisContext, AnalysisResult, Model } from './analysis'
import type { ApiEntry } from './api'
import {
  analysisFor,
  completionsIn,
  contextFor,
  contextKey,
  hoverIn,
  toScriptDiagnostics,
  type ScriptHover,
  type ScriptOptions,
} from './offsets'
import { entrySummary, streamNote, stripSnippet, type CompletionItem, type CompletionKind } from './service'

const LINT_DELAY = 220
const COMPLETION_DELAY = 150
const MAX_OPTIONS = 60

function el<K extends keyof HTMLElementTagNameMap>(tag: K, className: string, text?: string): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag)
  if (className) node.className = className
  if (text !== undefined) node.textContent = text
  return node
}

const CHIP = 'inline-flex h-5 max-w-full items-center truncate rounded-md px-2 font-mono text-xs leading-none'

function chip(text: string, tone: 'accent' | 'warning', title?: string): HTMLElement {
  const node = el('span', `${CHIP} ${tone === 'accent' ? 'bg-tone-accent text-accent' : 'bg-tone-warning text-warning'}`, text)
  if (title) node.title = title
  return node
}

function appendInline(parent: HTMLElement, text: string) {
  text.split('`').forEach((part, i) => {
    if (part === '') return
    if (i % 2 === 1) parent.append(el('code', 'font-mono text-accent', part))
    else parent.append(part.replaceAll('*', ''))
  })
}

function cardRoot(padded: boolean): HTMLElement {
  return el('div', `${padded ? 'px-2.5 py-2 ' : ''}max-w-[440px] font-sans text-sm leading-normal text-text`)
}

function cardTitle(text: string): HTMLElement {
  return el('div', 'font-mono font-semibold break-words whitespace-pre-wrap text-text', text)
}

/**
 * The symbol card for a prelude entry: signature, summary, required capability,
 * yields / maintenance badges, and llm.stream note.
 */
export function entryCard(entry: ApiEntry, padded = true): HTMLElement {
  const root = cardRoot(padded)
  root.append(cardTitle(entry.signature))
  root.append(el('div', 'mt-1 text-muted', entrySummary(entry)))
  if (entry.capability || entry.yields || entry.maintenance) {
    const row = el('div', 'mt-1.5 flex flex-wrap items-center gap-1')
    if (entry.capability) {
      row.append(chip(i18n('script.doc.capability', { capability: entry.capability }), 'accent', i18n('script.doc.capabilityTip')))
    }
    if (entry.yields) {
      row.append(chip(i18n('script.doc.yields'), 'warning', i18n('script.doc.yieldsTip')))
    }
    if (entry.maintenance) {
      row.append(chip(i18n('script.doc.maintenance'), 'warning'))
    }
    root.append(row)
  }
  if (entry.path === 'llm.stream') {
    const note = el('div', 'mt-1.5 text-muted')
    appendInline(note, streamNote())
    root.append(note)
  }
  return root
}

/** The hover card: entry card for prelude symbols, title and body otherwise. */
export function hoverCard(hover: ScriptHover): HTMLElement {
  if (hover.entry) return entryCard(hover.entry)
  const root = cardRoot(true)
  const [title, ...body] = hover.markdown.split('\n\n')
  root.append(cardTitle(title.replaceAll('**', '')))
  for (const paragraph of body) {
    const p = el('div', 'mt-1 text-muted')
    appendInline(p, paragraph)
    root.append(p)
  }
  return root
}

const completionType: Record<CompletionKind, string> = {
  function: 'function',
  field: 'property',
  variable: 'variable',
  module: 'namespace',
  snippet: 'text',
}

function insertText(item: CompletionItem): string {
  return stripSnippet(item.insert) || item.label
}

function toCompletion(item: CompletionItem): Completion {
  const entry = item.entry
  return {
    label: item.label,
    type: completionType[item.kind],
    detail: item.detail || undefined,
    info: entry ? () => entryCard(entry, false) : undefined,
    apply: insertText(item),
  }
}

function inCommentOrString(model: Model, off: number): boolean {
  for (const c of model.comments) {
    if (c.start >= off) break
    if (off <= c.end) return true
  }
  for (const t of model.tokens) {
    if (t.start >= off) break
    if (t.kind !== 'string') continue
    if (off < t.end) return true
    if (off === t.end) {
      const first = t.lit[0]
      const last = t.lit[t.lit.length - 1]
      const closed = t.lit.length >= 2 && last === first
      if (!closed) return true
    }
  }
  return false
}

class GhostWidget extends WidgetType {
  readonly text: string

  constructor(text: string) {
    super()
    this.text = text
  }

  eq(other: GhostWidget) {
    return other.text === this.text
  }

  toDOM() {
    const span = el('span', 'cm-ghost', this.text)
    span.setAttribute('aria-hidden', 'true')
    return span
  }

  ignoreEvent() {
    return false
  }
}

function ghostDecorations(state: EditorState): DecorationSet {
  if (completionStatus(state) !== 'active') return Decoration.none
  const selected = selectedCompletion(state)
  const range = state.selection.main
  if (!selected || !range.empty) return Decoration.none
  const text = typeof selected.apply === 'string' ? selected.apply : selected.label
  const line = state.doc.lineAt(range.head)
  const before = line.text.slice(0, range.head - line.from)
  const prefix = /[A-Za-z0-9_$]*$/.exec(before)?.[0] ?? ''
  if (!text.toLowerCase().startsWith(prefix.toLowerCase())) return Decoration.none
  const rest = text.slice(prefix.length).split('\n')[0]
  if (rest === '') return Decoration.none
  return Decoration.set([Decoration.widget({ widget: new GhostWidget(rest), side: 1 }).range(range.head)])
}

const ghostText = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet

    constructor(view: EditorView) {
      this.decorations = ghostDecorations(view.state)
    }

    update(update: ViewUpdate) {
      this.decorations = ghostDecorations(update.state)
    }
  },
  { decorations: (plugin) => plugin.decorations },
)

const theme = EditorView.theme({
  '.cm-gutter-lint': { width: '10px' },
  '.cm-gutter-lint .cm-gutterElement': { padding: '0 0 0 4px' },
  '.cm-lint-marker': {
    content: 'normal',
    boxSizing: 'border-box',
    width: '100%',
    height: '100%',
    borderLeft: '3px solid var(--af-warning)',
    cursor: 'default',
  },
  '.cm-lint-marker-error': { content: 'normal', borderLeftColor: 'var(--af-danger)' },
  '.cm-lint-marker-warning': { content: 'normal', borderLeftColor: 'var(--af-warning)' },
  '.cm-lint-marker-info': { content: 'normal', borderLeftColor: 'var(--af-accent)' },
  '.cm-lint-marker-hint': { content: 'normal', borderLeftColor: 'var(--af-faint)' },
  '.cm-lintPoint': { '&:after': { borderBottomColor: 'var(--af-danger)' } },
  '.cm-lintPoint-warning': { '&:after': { borderBottomColor: 'var(--af-warning)' } },
  '.cm-lintPoint-info': { '&:after': { borderBottomColor: 'var(--af-accent)' } },
  '.cm-lintPoint-hint': { '&:after': { borderBottomColor: 'var(--af-faint)' } },
  '.cm-lintRange-hint': {
    backgroundImage: 'none',
    textDecoration: 'underline dotted var(--af-faint)',
    textUnderlineOffset: '3px',
  },
  '.cm-lintRange-active': { backgroundColor: 'color-mix(in srgb, var(--af-warning) 18%, transparent)' },
  '.cm-diagnostic-hint': { borderLeftColor: 'var(--af-faint)' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': { maxWidth: 'min(560px, 95vw)' },
  '.cm-completionIcon': { color: 'var(--af-accent)', opacity: '0.9' },
  '.cm-completionMatchedText': { textDecoration: 'none', color: 'var(--af-accent)' },
  '.cm-ghost': { fontStyle: 'italic', pointerEvents: 'none' },
})

/** The three sources behind the extension, usable without a view. */
export interface ScriptSources {
  diagnostics(state: EditorState): CmDiagnostic[]
  completions(context: CompletionContext): CompletionResult | null
  hover(state: EditorState, pos: number, side: -1 | 1): ScriptHover | null
}

export function scriptSources(ctx: AnalysisContext): ScriptSources {
  const analyses = new WeakMap<Text, { lang: Lang; result: AnalysisResult }>()
  const analysisOf = (state: EditorState): AnalysisResult => {
    const lang = currentLang()
    let hit = analyses.get(state.doc)
    if (!hit || hit.lang !== lang) {
      hit = { lang, result: analysisFor(state.doc.toString(), ctx) }
      analyses.set(state.doc, hit)
    }
    return hit.result
  }

  return {
    diagnostics: (state) =>
      toScriptDiagnostics(analysisOf(state)).map((d) => ({
        from: d.from,
        to: d.to,
        severity: d.severity,
        message: d.message,
      })),

    completions: (context) => {
      const model = analysisOf(context.state).model
      const found = completionsIn(model, context.pos)
      if (!context.explicit) {
        if (found.prefix === '' && found.path === '') return null
        const before = found.from > 0 ? model.source[found.from - 1] : ''
        if (before === '.' && found.path === '') return null
        if (inCommentOrString(model, model.text.fromUtf16(context.pos))) return null
      }
      if (found.items.length === 0) return null
      const matched = [0, found.prefix.length]
      return {
        from: found.from,
        to: found.to,
        options: found.items.slice(0, MAX_OPTIONS).map(toCompletion),
        filter: false,
        getMatch: () => matched,
      }
    },

    hover: (state, pos, side) => {
      const at = side < 0 ? pos - 1 : pos
      if (at < 0) return null
      const hover = hoverIn(analysisOf(state).model, at)
      if (!hover || at < hover.from || at >= hover.to) return null
      let overDiagnostic = false
      forEachDiagnostic(state, (_d, from, to) => {
        if (at >= from && at < Math.max(to, from + 1)) overDiagnostic = true
      })
      return overDiagnostic ? null : hover
    },
  }
}

/** Builds the CodeMirror extension set for an analyzer context. */
export function scriptExtension(ctx: AnalysisContext): Extension {
  const sources = scriptSources(ctx)

  const hoverSource = (view: EditorView, pos: number, side: -1 | 1): Tooltip | null => {
    const hover = sources.hover(view.state, pos, side)
    if (!hover) return null
    return { pos: hover.from, end: hover.to, create: () => ({ dom: hoverCard(hover) }) }
  }

  return [
    Prec.high(lintGutter()),
    linter((view) => sources.diagnostics(view.state), { delay: LINT_DELAY }),
    autocompletion({
      override: [sources.completions],
      activateOnTyping: true,
      activateOnTypingDelay: COMPLETION_DELAY,
      maxRenderedOptions: MAX_OPTIONS,
    }),
    ghostText,
    hoverTooltip(hoverSource, { hideOnChange: true }),
    theme,
  ]
}

const built = new Map<string, Extension>()

/** Cached CodeMirror extension set for TypeScript/JS loop and plugin editors. */
export function scriptSupport(opts: ScriptOptions): Extension {
  const ctx = contextFor(opts)
  const key = contextKey(ctx)
  let ext = built.get(key)
  if (!ext) {
    ext = scriptExtension({
      preludeVersion: ctx.preludeVersion,
      capabilities: ctx.capabilities ? [...ctx.capabilities] : null,
      requireLoop: ctx.requireLoop,
    })
    if (built.size >= 32) built.clear()
    built.set(key, ext)
  }
  return ext
}
