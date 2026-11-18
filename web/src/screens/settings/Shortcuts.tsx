// The keyboard reference. The destination keys are read from the same list
// the shell binds them from (src/nav.ts), so the table cannot drift from what
// the keys do.
import { Kbd, Section } from '@/components/ui'
import { ALT, MOD, NAV, digitShortcut } from '@/nav'

interface Shortcut {
  keys: string[]
  what: string
}

const DESTINATIONS: Shortcut[] = NAV.filter((it) => it.digit !== undefined)
  .sort((a, b) => (a.digit ?? 0) - (b.digit ?? 0))
  .map((it) => ({ keys: [digitShortcut(it.digit!)], what: 'Go to ' + it.label }))

const ACTIONS: Shortcut[] = [
  { keys: [`${MOD}+K`, `${MOD}+Shift+P`], what: 'Open the command palette' },
  { keys: [`${ALT}+N`], what: 'Create a new agent' },
  { keys: [`${ALT}+R`], what: 'Refresh the current screen' },
  { keys: [`${MOD}+S`], what: 'Save: apply changes in Builder, validate and save in Config' },
  { keys: [`${MOD}+Enter`], what: 'Send message in Playground' },
  { keys: ['Ctrl+Space'], what: 'Trigger Luau completion' },
  { keys: ['F1'], what: 'Open docs' },
  { keys: ['Esc'], what: 'Return to previous view' },
]

function Rows({ title, rows }: { title: string; rows: Shortcut[] }) {
  return (
    <div className="min-w-0">
      <div className="pb-1 text-xs font-semibold text-faint">{title}</div>
      <table className="w-full border-collapse text-sm">
        <tbody>
          {rows.map((r) => (
            <tr key={r.what} className="border-b border-line/70 last:border-b-0">
              <td className="w-[150px] whitespace-nowrap py-1.5 pr-3 align-middle">
                <span className="inline-flex items-center gap-1">
                  {r.keys.map((k, i) => (
                    <span key={k} className="inline-flex items-center gap-1">
                      {i > 0 && <span className="text-xs text-faint">or</span>}
                      <Kbd>{k}</Kbd>
                    </span>
                  ))}
                </span>
              </td>
              <td className="py-1.5 align-middle text-text">{r.what}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

export function ShortcutsSection() {
  return (
    <Section title="Keyboard shortcuts" subtitle="Navigate workspaces and trigger common editor actions from the keyboard.">
      <div className="grid gap-x-8 gap-y-4 md:grid-cols-2">
        <Rows title="Workspace views" rows={DESTINATIONS} />
        <Rows title="Actions" rows={ACTIONS} />
      </div>
      <p className="mt-3 border-t border-line pt-2.5 text-xs leading-snug text-muted">
        These differ from the desktop app, which uses Ctrl+N, Ctrl+1…9 and F5. A browser keeps those for a new window, switching tabs and reloading
        the page, so here a new agent is {ALT}+N, the views are {ALT}+1…9 and refresh is {ALT}+R.
      </p>
    </Section>
  )
}
