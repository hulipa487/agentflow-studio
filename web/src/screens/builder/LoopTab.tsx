// The Loop tab: the Luau editor with its language intelligence, a one-line
// verdict beside the title, and the issues as a list that jumps to the line.
// Diagnostics are also drawn in the editor itself by the luau extension.
import { useEffect, useRef, useState } from 'react'
import type { Extension } from '@codemirror/state'
import { EditorView } from '@codemirror/view'
import { startCompletion } from '@codemirror/autocomplete'
import { ChevronDown, ChevronRight, List } from 'lucide-react'
import { Chip, CopyButton, Dot, IconButton, cx, toneText, type Tone } from '@/components/ui'
import { CodeEditor } from '@/components/ui/CodeEditor'
import type { LuauDiagnostic, Severity } from '@/lua'
import { EditorFrame } from './EditorFrame'
import { loopStatus } from './model'

const SEVERITY_TONE: Record<Severity, Tone> = { error: 'danger', warning: 'warning', info: 'accent', hint: 'neutral' }
const SEVERITY_ORDER: Record<Severity, number> = { error: 0, warning: 1, info: 2, hint: 3 }

export function LoopTab({
  source,
  onChange,
  extensions,
  diagnostics,
  checking,
  failure,
  active,
}: {
  source: string
  onChange: (text: string) => void
  extensions: Extension[]
  diagnostics: LuauDiagnostic[]
  /** True while an edit is waiting for the next analysis pass. */
  checking: boolean
  /** Set when the analyzer itself failed. */
  failure: string
  /** Whether this tab is the visible one. */
  active: boolean
}) {
  const view = useRef<EditorView | null>(null)
  const [open, setOpen] = useState(true)
  const status = loopStatus(diagnostics, checking, failure)

  // Selecting the tab puts the caret back in the loop, as the desktop does.
  useEffect(() => {
    if (!active) return
    const v = view.current
    if (!v) return
    v.requestMeasure()
    v.focus()
  }, [active])

  const jump = (d: LuauDiagnostic) => {
    const v = view.current
    if (!v) return
    // The list can be one debounce behind the document; stay inside it.
    const len = v.state.doc.length
    const from = Math.min(Math.max(d.from, 0), len)
    const to = Math.min(Math.max(d.to, from), len)
    v.dispatch({ selection: { anchor: from, head: to }, effects: EditorView.scrollIntoView(from, { y: 'center' }) })
    v.focus()
  }

  const issues = [...diagnostics].sort((a, b) => SEVERITY_ORDER[a.severity] - SEVERITY_ORDER[b.severity] || a.from - b.from)

  return (
    <EditorFrame
      title="Loop (Luau)"
      hint="Luau · inline diagnostics · Ctrl+Space for API"
      actions={
        <>
          <span className="mr-1 flex min-w-0 items-center gap-1.5" role="status">
            <Dot tone={status.tone} />
            <span className={cx('truncate text-xs', status.tone === 'neutral' ? 'text-muted' : toneText(status.tone))}>{status.text}</span>
          </span>
          <CopyButton text={source} what="loop" size="md" />
          <IconButton
            icon={List}
            label="Prelude API (Ctrl+Space)"
            onClick={() => {
              const v = view.current
              if (!v) return
              v.focus()
              startCompletion(v)
            }}
          />
        </>
      }
      footer={
        issues.length > 0 ? (
          <div className="shrink-0 border-t border-line">
            <button
              type="button"
              onClick={() => setOpen((o) => !o)}
              aria-expanded={open}
              className="flex h-7 w-full items-center gap-1.5 px-3 text-left text-[10px] font-semibold uppercase tracking-[0.06em] text-faint hover:text-text"
            >
              {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
              Issues
              <span className="font-mono normal-case tracking-normal">{issues.length}</span>
            </button>
            {open && (
              <ul className="max-h-40 overflow-y-auto pb-1.5">
                {issues.map((d, i) => (
                  <li key={`${d.from}:${d.to}:${d.code ?? ''}:${i}`}>
                    <button
                      type="button"
                      onClick={() => jump(d)}
                      title="Jump to this line"
                      className="flex w-full items-baseline gap-2 px-3 py-1 text-left text-xs hover:bg-hover focus-visible:bg-hover"
                    >
                      <Dot tone={SEVERITY_TONE[d.severity]} className="translate-y-[-1px] self-center" />
                      <span className="w-12 shrink-0 font-mono text-[11px] text-faint">L{d.line}</span>
                      <span className="min-w-0 flex-1 break-words text-text">{d.message}</span>
                      {d.code && <Chip>{d.code}</Chip>}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : undefined
      }
    >
      <CodeEditor
        language="lua"
        value={source}
        onChange={onChange}
        extensions={extensions}
        autoFocus
        onReady={(v) => {
          view.current = v
        }}
        className="flex-1"
      />
    </EditorFrame>
  )
}
