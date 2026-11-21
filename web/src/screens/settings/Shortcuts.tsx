// The keyboard reference rendered inside Settings; shares its definitions
// with the global `?` modal in components/shell/ShortcutsModal.tsx.
import { Section } from '@/components/ui'
import { ShortcutsGrid } from '@/components/shell/ShortcutsModal'
import { t } from '@/i18n'

export function ShortcutsSection() {
  return (
    <Section title={t('settings.shortcuts')} subtitle={t('settings.shortcutsHint')}>
      <ShortcutsGrid />
    </Section>
  )
}
