// Shared keyboard shortcut reference grid + global `?` modal.
import { Button, Kbd, Modal } from '@/components/ui'
import { t } from '@/i18n'
import { ALT, MOD, NAV, digitShortcut } from '@/nav'
import { useUI } from '@/state/ui'

export interface ShortcutItem {
  keys: string[]
  what: string
}

function destinations(): ShortcutItem[] {
  return NAV.filter((it) => it.digit !== undefined)
    .sort((a, b) => (a.digit ?? 0) - (b.digit ?? 0))
    .map((it) => ({ keys: [digitShortcut(it.digit!)], what: t('shell.palette.goTo', { screen: t(it.labelKey) }) }))
}

const ACTIONS: { keys: string[]; whatKey: string }[] = [
  { keys: [`${MOD}+K`, `${MOD}+Shift+P`], whatKey: 'settings.shortcutPalette' },
  { keys: [`${ALT}+N`], whatKey: 'settings.shortcutNewAgent' },
  { keys: [`${ALT}+R`], whatKey: 'settings.shortcutRefresh' },
  { keys: [`${MOD}+S`], whatKey: 'settings.shortcutSaveWeb' },
  { keys: [`${MOD}+Enter`], whatKey: 'settings.shortcutSend' },
  { keys: ['Ctrl+Space'], whatKey: 'settings.shortcutCompletion' },
  { keys: ['F1'], whatKey: 'settings.shortcutDocs' },
  { keys: ['?'], whatKey: 'settings.shortcutHelp' },
  { keys: ['Esc'], whatKey: 'settings.shortcutBack' },
]

function Rows({ title, rows }: { title: string; rows: ShortcutItem[] }) {
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
                      {i > 0 && <span className="text-xs text-faint">{t('settings.shortcutOr')}</span>}
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

export function ShortcutsGrid() {
  return (
    <>
      <div className="grid gap-x-8 gap-y-4 md:grid-cols-2">
        <Rows title={t('settings.shortcutViews')} rows={destinations()} />
        <Rows title={t('settings.shortcutActions')} rows={ACTIONS.map((a) => ({ keys: a.keys, what: t(a.whatKey) }))} />
      </div>
      <p className="mt-3 border-t border-line pt-2.5 text-xs leading-snug text-muted">{t('settings.shortcutsBrowserNote', { alt: ALT })}</p>
    </>
  )
}

export function ShortcutsModal() {
  const open = useUI((s) => s.shortcutsOpen)
  const setOpen = useUI((s) => s.setShortcuts)
  return (
    <Modal
      open={open}
      onClose={() => setOpen(false)}
      title={t('settings.shortcuts')}
      subtitle={t('settings.shortcutsHint')}
      width={720}
      footer={
        <Button variant="primary" onClick={() => setOpen(false)}>
          {t('common.close')}
        </Button>
      }
    >
      <ShortcutsGrid />
    </Modal>
  )
}
