// A CodeMirror 6 editor themed from the design tokens. It is the web
// counterpart of the desktop app's purpose-built editor widget: line numbers,
// highlighting, bracket pairing, auto-indent, search, multi-cursor, undo/redo
// and the usual IDE keys. Language intelligence for Luau loops (diagnostics,
// completion, hover) is supplied by the caller through `extensions`.
import { useEffect, useRef, useState } from 'react'
import { Compartment, EditorSelection, EditorState, RangeSetBuilder, type ChangeSpec, type Extension } from '@codemirror/state'
import {
  Decoration,
  EditorView,
  ViewPlugin,
  crosshairCursor,
  drawSelection,
  type Command,
  type DecorationSet,
  type ViewUpdate,
  dropCursor,
  highlightActiveLine,
  highlightActiveLineGutter,
  highlightSpecialChars,
  keymap,
  lineNumbers,
  placeholder as cmPlaceholder,
  rectangularSelection,
} from '@codemirror/view'
import { defaultKeymap, history, historyKeymap, indentWithTab } from '@codemirror/commands'
import {
  HighlightStyle,
  StreamLanguage,
  bracketMatching,
  foldGutter,
  foldService,
  foldKeymap,
  indentOnInput,
  indentUnit,
  syntaxHighlighting,
} from '@codemirror/language'
import { acceptCompletion, closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete'
import { highlightSelectionMatches, openSearchPanel, search, searchKeymap } from '@codemirror/search'
import { lintKeymap } from '@codemirror/lint'
import { lua } from '@codemirror/legacy-modes/mode/lua'
import { yaml } from '@codemirror/lang-yaml'
import { markdown } from '@codemirror/lang-markdown'
import { json } from '@codemirror/lang-json'
import { tags as t } from '@lezer/highlight'
import { cx } from './index'

export type CodeLanguage = 'lua' | 'yaml' | 'markdown' | 'json' | 'text'

// Colors come from the tokens, so the editor follows the theme and palette
// without being rebuilt.
const highlight = HighlightStyle.define([
  { tag: [t.keyword, t.controlKeyword, t.operatorKeyword, t.modifier], color: 'var(--af-accent2)' },
  { tag: [t.string, t.special(t.string), t.regexp], color: 'var(--af-success)' },
  { tag: [t.number, t.bool, t.null, t.atom], color: 'var(--af-warning)' },
  { tag: [t.comment, t.lineComment, t.blockComment, t.meta], color: 'var(--af-faint)', fontStyle: 'italic' },
  { tag: [t.function(t.variableName), t.function(t.propertyName), t.standard(t.variableName)], color: 'var(--af-accent)' },
  { tag: [t.propertyName, t.attributeName, t.definition(t.propertyName)], color: 'var(--af-accent)' },
  { tag: [t.typeName, t.className, t.namespace], color: 'var(--af-accent)' },
  { tag: [t.operator, t.punctuation, t.bracket, t.separator], color: 'var(--af-muted)' },
  { tag: t.variableName, color: 'var(--af-text)' },
  { tag: [t.heading, t.strong], fontWeight: '650', color: 'var(--af-text)' },
  { tag: t.emphasis, fontStyle: 'italic' },
  { tag: [t.link, t.url], color: 'var(--af-accent)', textDecoration: 'underline' },
  { tag: t.monospace, color: 'var(--af-accent)' },
  { tag: t.invalid, color: 'var(--af-danger)' },
])

// The editor chrome, from the same tokens. It is an EditorView theme rather
// than stylesheet rules because CodeMirror injects its base styles unlayered,
// where Tailwind's layered CSS cannot override them.
const chrome = EditorView.theme({
  '&': { height: '100%', fontSize: '12.5px', backgroundColor: 'var(--af-bg)', color: 'var(--af-text)' },
  '&.cm-focused': { outline: 'none' },
  '.cm-scroller': { fontFamily: 'var(--font-mono)', lineHeight: '1.6' },
  '.cm-content': { caretColor: 'var(--af-accent)' },
  '.cm-gutters': { backgroundColor: 'var(--af-bg)', color: 'var(--af-faint)', borderRight: '1px solid var(--af-line)' },
  '.cm-activeLineGutter': { backgroundColor: 'var(--af-surface2)', color: 'var(--af-muted)' },
  '.cm-activeLine': { backgroundColor: 'color-mix(in srgb, var(--af-surface2) 55%, transparent)' },
  '.cm-cursor, .cm-dropCursor': { borderLeft: '2px solid var(--af-accent)' },
  '.cm-selectionBackground': { backgroundColor: 'color-mix(in srgb, var(--af-accent) 18%, transparent)' },
  '&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground': {
    backgroundColor: 'color-mix(in srgb, var(--af-accent) 28%, transparent)',
  },
  '.cm-selectionMatch': { backgroundColor: 'color-mix(in srgb, var(--af-accent2) 22%, transparent)' },
  '.cm-searchMatch': { backgroundColor: 'color-mix(in srgb, var(--af-warning) 30%, transparent)', outline: '1px solid var(--af-warning)' },
  '.cm-searchMatch.cm-searchMatch-selected': { backgroundColor: 'color-mix(in srgb, var(--af-accent) 35%, transparent)' },
  '&.cm-focused .cm-matchingBracket, .cm-matchingBracket': {
    backgroundColor: 'color-mix(in srgb, var(--af-accent) 22%, transparent)',
    outline: '1px solid var(--af-line)',
  },
  '&.cm-focused .cm-nonmatchingBracket': { backgroundColor: 'color-mix(in srgb, var(--af-danger) 25%, transparent)' },
  '.cm-foldPlaceholder': { backgroundColor: 'var(--af-surface2)', border: '1px solid var(--af-line)', color: 'var(--af-muted)' },
  '.cm-placeholder': { color: 'var(--af-faint)' },
  '.cm-indent': {
    backgroundImage: 'linear-gradient(to right, var(--af-line) 1px, transparent 1px)',
    backgroundSize: '2ch 100%',
    padding: '0.3em 0',
  },

  '.cm-tooltip': {
    backgroundColor: 'var(--af-float)',
    border: '1px solid var(--af-line)',
    borderRadius: '8px',
    boxShadow: 'var(--af-shadow)',
    color: 'var(--af-text)',
    overflow: 'hidden',
  },
  '.cm-tooltip.cm-tooltip-autocomplete > ul': { fontFamily: 'var(--font-mono)', fontSize: '12px', maxHeight: '260px' },
  '.cm-tooltip.cm-tooltip-autocomplete > ul > li': { padding: '3px 8px' },
  '.cm-tooltip-autocomplete ul li[aria-selected]': { backgroundColor: 'var(--af-accent-dim)', color: 'var(--af-text)' },
  '.cm-completionDetail': { color: 'var(--af-faint)', fontStyle: 'normal', marginLeft: '10px' },
  '.cm-completionMatchedText': { textDecoration: 'none', color: 'var(--af-accent)', fontWeight: '600' },
  '.cm-tooltip.cm-completionInfo': { padding: '8px 10px', maxWidth: '360px', fontFamily: 'var(--font-sans)', fontSize: '12px', lineHeight: '1.5' },
  '.cm-tooltip-hover, .cm-tooltip-lint': { padding: '0', fontSize: '12px', maxWidth: '460px' },
  '.cm-diagnostic': { padding: '6px 10px', borderLeftWidth: '3px', fontFamily: 'var(--font-sans)', whiteSpace: 'pre-wrap' },
  '.cm-diagnostic-error': { borderLeftColor: 'var(--af-danger)' },
  '.cm-diagnostic-warning': { borderLeftColor: 'var(--af-warning)' },
  '.cm-diagnostic-info': { borderLeftColor: 'var(--af-accent)' },
  '.cm-lintRange-error': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy var(--af-danger)',
    textDecorationSkipInk: 'none',
    textUnderlineOffset: '3px',
  },
  '.cm-lintRange-warning': {
    backgroundImage: 'none',
    textDecoration: 'underline wavy var(--af-warning)',
    textDecorationSkipInk: 'none',
    textUnderlineOffset: '3px',
  },
  '.cm-lintRange-info, .cm-lintRange-hint': {
    backgroundImage: 'none',
    textDecoration: 'underline dotted var(--af-accent)',
    textUnderlineOffset: '3px',
  },

  '.cm-panels': { backgroundColor: 'var(--af-surface)', color: 'var(--af-text)' },
  '.cm-panels.cm-panels-top': { borderBottom: '1px solid var(--af-line)' },
  '.cm-panels.cm-panels-bottom': { borderTop: '1px solid var(--af-line)' },
  '.cm-panel input, .cm-panel button, .cm-panel label': { fontFamily: 'var(--font-sans)', fontSize: '12px' },
  '.cm-textfield': { backgroundColor: 'var(--af-bg)', border: '1px solid var(--af-line)', borderRadius: '6px', color: 'var(--af-text)' },
  '.cm-button': {
    backgroundColor: 'var(--af-surface2)',
    backgroundImage: 'none',
    border: '1px solid var(--af-line)',
    borderRadius: '6px',
    color: 'var(--af-text)',
  },
})

// The legacy Lua mode has no syntax tree to fold on, so Lua folds by
// indentation: a line folds over the deeper-indented lines below it.
const indentFold = foldService.of((state, lineStart) => {
  const line = state.doc.lineAt(lineStart)
  if (!line.text.trim()) return null
  const indent = /^\s*/.exec(line.text)![0].length
  let end = -1
  for (let n = line.number + 1; n <= state.doc.lines; n++) {
    const l = state.doc.line(n)
    if (!l.text.trim()) continue
    if (/^\s*/.exec(l.text)![0].length <= indent) break
    end = l.to
  }
  return end > line.to ? { from: line.to, to: end } : null
})

// Indent guides: the leading spaces of each line get a mark whose background
// draws a hairline at the start of every indent level.
const indentMark = Decoration.mark({ class: 'cm-indent' })
function guides(view: EditorView): DecorationSet {
  const b = new RangeSetBuilder<Decoration>()
  for (const { from, to } of view.visibleRanges) {
    for (let pos = from; pos <= to; ) {
      const line = view.state.doc.lineAt(pos)
      const lead = /^ +/.exec(line.text)
      if (lead && line.text.trim()) b.add(line.from, line.from + lead[0].length, indentMark)
      pos = line.to + 1
    }
  }
  return b.finish()
}
const indentGuides = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet
    constructor(view: EditorView) {
      this.decorations = guides(view)
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged) this.decorations = guides(u.view)
    }
  },
  { decorations: (v) => v.decorations },
)

/** Ctrl+U / Ctrl+Shift+U: lower- or upper-case each selection, or the word at a caret. */
function changeCase(upper: boolean): Command {
  return (view) => {
    const tr = view.state.changeByRange((range) => {
      const target = range.empty ? view.state.wordAt(range.head) : range
      if (!target) return { range }
      const text = view.state.sliceDoc(target.from, target.to)
      return {
        changes: { from: target.from, to: target.to, insert: upper ? text.toUpperCase() : text.toLowerCase() },
        range: EditorSelection.range(range.anchor, range.head),
      }
    })
    view.dispatch(view.state.update(tr, { scrollIntoView: true, userEvent: 'input' }))
    return true
  }
}

/** Ctrl+J: join each selection's lines (or the caret's line and the next) with one space. */
const joinLines: Command = (view) => {
  const { state } = view
  const changes: ChangeSpec[] = []
  const seen = new Set<number>()
  for (const r of state.selection.ranges) {
    const first = state.doc.lineAt(r.from).number
    const last = state.doc.lineAt(r.to).number
    const end = last === first ? first + 1 : last
    for (let n = first; n < end && n < state.doc.lines; n++) {
      if (seen.has(n)) continue
      seen.add(n)
      const line = state.doc.line(n)
      const next = state.doc.line(n + 1)
      const lead = /^\s*/.exec(next.text)![0].length
      const sep = /\s$/.test(line.text) || !next.text.trim() || !line.text ? '' : ' '
      changes.push({ from: line.to, to: next.from + lead, insert: sep })
    }
  }
  if (changes.length === 0) return false
  view.dispatch({ changes, scrollIntoView: true, userEvent: 'input' })
  return true
}

const LANGUAGE_LABEL: Record<CodeLanguage, string> = { lua: 'Luau', yaml: 'YAML', markdown: 'Markdown', json: 'JSON', text: 'Plain text' }

interface Caret {
  line: number
  col: number
  selections: number
  selected: number
}

function caretOf(state: EditorState): Caret {
  const main = state.selection.main
  const line = state.doc.lineAt(main.head)
  return { line: line.number, col: main.head - line.from + 1, selections: state.selection.ranges.length, selected: main.to - main.from }
}

function languageExtension(lang: CodeLanguage): Extension {
  switch (lang) {
    case 'lua':
      return [StreamLanguage.define(lua), indentFold]
    case 'yaml':
      return yaml()
    case 'markdown':
      return markdown()
    case 'json':
      return json()
    default:
      return []
  }
}

export interface CodeEditorProps {
  value: string
  onChange?: (value: string) => void
  language?: CodeLanguage
  readOnly?: boolean
  /** Extra CodeMirror extensions: linters, completion sources, hover tooltips. */
  extensions?: Extension[]
  placeholder?: string
  lineWrapping?: boolean
  autoFocus?: boolean
  /** Receives the view once, for callers that need to drive it (focus, scroll). */
  onReady?: (view: EditorView) => void
  /** The status line under the editor (caret, selections, language); on for editable documents. */
  status?: boolean
  /** The wrapper's classes; give it a height (h-full, h-64, min-h-0 flex-1). */
  className?: string
}

export function CodeEditor({
  value,
  onChange,
  language = 'text',
  readOnly = false,
  extensions,
  placeholder,
  lineWrapping = false,
  autoFocus,
  onReady,
  status,
  className,
}: CodeEditorProps) {
  const showStatus = status ?? !readOnly
  const [caret, setCaret] = useState<Caret>({ line: 1, col: 1, selections: 1, selected: 0 })
  // Alt+Z flips soft wrap; a new lineWrapping prop resets it.
  const [wrap, setWrap] = useState(lineWrapping)
  useEffect(() => setWrap(lineWrapping), [lineWrapping])
  const wrapRef = useRef(wrap)
  wrapRef.current = wrap
  const host = useRef<HTMLDivElement>(null)
  const view = useRef<EditorView | null>(null)
  const onChangeRef = useRef(onChange)
  onChangeRef.current = onChange
  const compartments = useRef({
    lang: new Compartment(),
    readOnly: new Compartment(),
    extra: new Compartment(),
    wrap: new Compartment(),
  })

  useEffect(() => {
    if (!host.current) return
    const c = compartments.current
    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        highlightSpecialChars(),
        history(),
        foldGutter(),
        drawSelection(),
        dropCursor(),
        EditorState.allowMultipleSelections.of(true),
        indentOnInput(),
        indentUnit.of('  '),
        EditorState.tabSize.of(2),
        chrome,
        syntaxHighlighting(highlight),
        bracketMatching(),
        closeBrackets(),
        rectangularSelection(),
        crosshairCursor(),
        indentGuides,
        search({ top: true }),
        highlightActiveLine(),
        highlightSelectionMatches(),
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...searchKeymap,
          ...historyKeymap,
          ...foldKeymap,
          ...completionKeymap,
          ...lintKeymap,
          // Tab takes the selected completion, as in the desktop editor; with
          // no menu open the binding declines and Tab indents.
          { key: 'Tab', run: acceptCompletion },
          indentWithTab,
          { key: 'Mod-u', run: changeCase(false) },
          { key: 'Mod-Shift-u', run: changeCase(true) },
          { key: 'Mod-j', run: joinLines },
          // The panel holds find and replace together; Ctrl+H opens it as on the desktop.
          { key: 'Mod-h', run: openSearchPanel },
          {
            key: 'Alt-z',
            run: () => {
              setWrap(!wrapRef.current)
              return true
            },
          },
        ]),
        placeholder ? cmPlaceholder(placeholder) : [],
        c.lang.of(languageExtension(language)),
        c.readOnly.of([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
        c.wrap.of(lineWrapping ? EditorView.lineWrapping : []),
        c.extra.of(extensions ?? []),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current?.(u.state.doc.toString())
          if (u.docChanged || u.selectionSet) setCaret(caretOf(u.state))
        }),
      ],
    })
    const v = new EditorView({ state, parent: host.current })
    view.current = v
    if (autoFocus) v.focus()
    onReady?.(v)
    return () => {
      v.destroy()
      view.current = null
    }
    // The view is created once; later prop changes are applied below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // An outside change to `value` (load, rebuild, revert) replaces the document;
  // the editor's own edits already match and are left alone.
  useEffect(() => {
    const v = view.current
    if (!v) return
    const current = v.state.doc.toString()
    // The document always uses \n; a value that differs only in its line
    // endings (a file saved on Windows) is the same text, not an edit to apply.
    if (value.replace(/\r\n?/g, '\n') !== current) {
      v.dispatch({ changes: { from: 0, to: current.length, insert: value } })
    }
  }, [value])

  useEffect(() => {
    view.current?.dispatch({ effects: compartments.current.lang.reconfigure(languageExtension(language)) })
  }, [language])

  useEffect(() => {
    view.current?.dispatch({
      effects: compartments.current.readOnly.reconfigure([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
    })
  }, [readOnly])

  useEffect(() => {
    view.current?.dispatch({ effects: compartments.current.wrap.reconfigure(wrap ? EditorView.lineWrapping : []) })
  }, [wrap])

  useEffect(() => {
    view.current?.dispatch({ effects: compartments.current.extra.reconfigure(extensions ?? []) })
  }, [extensions])

  const facts = [`Ln ${caret.line}, Col ${caret.col}`]
  if (caret.selections > 1) facts.push(`${caret.selections} selections`)
  else if (caret.selected > 0) facts.push(`${caret.selected} selected`)
  facts.push(LANGUAGE_LABEL[language], 'Spaces: 2')
  if (wrap) facts.push('Wrap')

  return (
    <div className={cx('flex min-h-0 flex-col overflow-hidden rounded-ctl border border-line bg-bg', className)}>
      <div ref={host} className="grid min-h-0 flex-1" />
      {showStatus && (
        <div className="shrink-0 truncate border-t border-line px-2.5 py-0.5 text-right font-mono text-xs text-faint" aria-live="off">
          {facts.join('  ·  ')}
        </div>
      )}
    </div>
  )
}
