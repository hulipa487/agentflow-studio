// The profile editor: a name and an ordered filter chain. The order here is
// the order the dispatcher runs the filters, so the chain is edited as a list
// that can be reordered rather than as free text.
import { useState } from 'react'
import { ArrowDown, ArrowUp, Plus, X } from 'lucide-react'
import type { Client } from '@/api/client'
import type { SafetyList, SafetyProfile } from '@/api/types'
import { Button, Chip, Field, IconButton, Input, Modal } from '@/components/ui'
import { splitList } from '@/lib/format'
import { CLASSIFIER_PREFIX, DEFAULT_CHAIN, EMPTY_CHAIN, appendFilters, buildProfileDraft, isClassifierFilter, moveFilter } from './helpers'
import { ChainView, EditorFooter, EditorNotice, useEditorActions } from './parts'

export function ProfileEditor({
  client,
  list,
  existing,
  onClose,
  onSaved,
  reload,
}: {
  client: Client
  /** The current safety list: the filters on offer and the mtime a save sends. */
  list: SafetyList
  /** Null creates a profile. */
  existing: SafetyProfile | null
  onClose: () => void
  onSaved: (list: SafetyList) => void
  reload: () => Promise<void>
}) {
  const creating = existing === null
  const [name, setName] = useState(existing?.name ?? '')
  const [chain, setChain] = useState<string[]>(existing ? (existing.filters ?? []) : DEFAULT_CHAIN)
  const [extra, setExtra] = useState('')

  const builtins = list.builtin_filters ?? []
  const classifierFilters = (list.classifiers ?? []).map((c) => CLASSIFIER_PREFIX + c.name)
  const available = [...builtins, ...classifierFilters]

  const actions = useEditorActions({
    build: () => buildProfileDraft(name, chain),
    validate: (d) => client.validateSafety({ kind: 'profile', creating, profile: d }),
    save: (d) => client.saveSafetyProfile(d.name, creating, d, list.mtime),
    validText: () => 'Valid. Save it to bind the chain on the next runtime start.',
    onSaved,
    reload,
  })
  const busy = actions.busy !== ''

  const addExtra = () => {
    const names = splitList(extra)
    if (names.length === 0) return
    setChain((c) => appendFilters(c, names))
    setExtra('')
  }

  // The runtime has the last word; this only flags what it will refuse.
  const kindOf = (f: string): { label: string; tone: 'neutral' | 'accent' | 'warning'; title?: string } => {
    if (builtins.includes(f)) return { label: 'builtin', tone: 'neutral' }
    if (classifierFilters.includes(f)) return { label: 'classifier', tone: 'accent' }
    if (isClassifierFilter(f)) return { label: 'no such classifier', tone: 'warning', title: 'Classifier filters need a saved classifier.' }
    return { label: 'unknown', tone: 'warning', title: 'Not a builtin filter of the running runtime.' }
  }

  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={640}
      title={creating ? 'New safety profile' : 'Edit ' + existing.name}
      subtitle="Profiles are core-owned: a loop cannot uninstall the chain. Changes bind when agentflow starts."
      footer={<EditorFooter actions={actions} onCancel={onClose} />}
    >
      <EditorNotice actions={actions} />
      <div className="flex flex-col gap-4">
        <Field label="Name" hint={'Immutable after creation. "default" and "none" are reserved references.'}>
          <Input mono value={name} onChange={(e) => setName(e.target.value)} placeholder="strict" disabled={!creating} autoFocus={creating} />
        </Field>

        <Field label="Filter chain" hint="Runs top to bottom. Order matters: move a filter with the arrows.">
          <ol className="overflow-hidden rounded-ctl border border-line bg-bg">
            {chain.length === 0 && <li className="px-2.5 py-2 font-mono text-[11.5px] text-faint">{EMPTY_CHAIN}</li>}
            {chain.map((f, i) => {
              const kind = kindOf(f)
              return (
                <li key={`${i}:${f}`} className="flex items-center gap-2 border-b border-line/70 py-0.5 pl-2.5 pr-1 last:border-b-0">
                  <span className="w-4 shrink-0 text-right font-mono text-[11px] tabular-nums text-faint">{i + 1}</span>
                  <span className="min-w-0 flex-1 truncate font-mono text-xs" title={f}>
                    {f}
                  </span>
                  <Chip tone={kind.tone} title={kind.title}>
                    {kind.label}
                  </Chip>
                  <span className="flex shrink-0 items-center">
                    <IconButton size="sm" icon={ArrowUp} label={`Move ${f} up`} disabled={i === 0} onClick={() => setChain((c) => moveFilter(c, i, -1))} />
                    <IconButton
                      size="sm"
                      icon={ArrowDown}
                      label={`Move ${f} down`}
                      disabled={i === chain.length - 1}
                      onClick={() => setChain((c) => moveFilter(c, i, 1))}
                    />
                    <IconButton size="sm" icon={X} tone="danger" label={`Remove ${f}`} onClick={() => setChain((c) => c.filter((_, at) => at !== i))} />
                  </span>
                </li>
              )
            })}
          </ol>
        </Field>

        <Field label="Available filters" hint="Click a name to append it, or add names comma-separated, left to right.">
          <div className="flex flex-wrap gap-1.5">
            {available.map((f) => (
              <Button
                key={f}
                size="sm"
                icon={Plus}
                className="font-mono"
                disabled={chain.includes(f)}
                title={chain.includes(f) ? 'Already in the chain' : undefined}
                onClick={() => setChain((c) => appendFilters(c, [f]))}
              >
                {f}
              </Button>
            ))}
          </div>
          <div className="mt-1 flex items-center gap-2">
            <Input
              mono
              value={extra}
              onChange={(e) => setExtra(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  addExtra()
                }
              }}
              placeholder="source-attribution, affect-guard"
              aria-label="Filters to add"
            />
            <Button onClick={addExtra} disabled={splitList(extra).length === 0}>
              Add
            </Button>
          </div>
        </Field>

        <Field label="Preview" hint="Classifier filters need a saved classifier below; builtin names come from the running runtime.">
          <div className="rounded-ctl border border-line bg-bg px-2.5 py-2">
            <ChainView filters={chain} />
          </div>
        </Field>
      </div>
    </Modal>
  )
}
