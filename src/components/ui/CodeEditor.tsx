// A CodeMirror 6 editor themed from the design tokens. It is the web
// counterpart of the desktop app's purpose-built editor widget: line numbers,
// highlighting, bracket pairing, auto-indent, search, multi-cursor, undo/redo
// and the usual IDE keys. Language intelligence for Luau loops (diagnostics,
// completion, hover) is supplied by the caller through `extensions`.
import { useEffect, useRef } from 'react'
import { Compartment, EditorState, type Extension } from '@codemirror/state'
import {
  EditorView,
  drawSelection,
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
  foldKeymap,
  indentOnInput,
  indentUnit,
  syntaxHighlighting,
} from '@codemirror/language'
import { closeBrackets, closeBracketsKeymap, completionKeymap } from '@codemirror/autocomplete'
import { highlightSelectionMatches, searchKeymap } from '@codemirror/search'
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

function languageExtension(lang: CodeLanguage): Extension {
  switch (lang) {
    case 'lua':
      return StreamLanguage.define(lua)
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
  className,
}: CodeEditorProps) {
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
          indentWithTab,
        ]),
        placeholder ? cmPlaceholder(placeholder) : [],
        c.lang.of(languageExtension(language)),
        c.readOnly.of([EditorState.readOnly.of(readOnly), EditorView.editable.of(!readOnly)]),
        c.wrap.of(lineWrapping ? EditorView.lineWrapping : []),
        c.extra.of(extensions ?? []),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current?.(u.state.doc.toString())
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
    view.current?.dispatch({ effects: compartments.current.wrap.reconfigure(lineWrapping ? EditorView.lineWrapping : []) })
  }, [lineWrapping])

  useEffect(() => {
    view.current?.dispatch({ effects: compartments.current.extra.reconfigure(extensions ?? []) })
  }, [extensions])

  return <div ref={host} className={cx('min-h-0 overflow-hidden rounded-ctl border border-line bg-bg', className)} />
}
