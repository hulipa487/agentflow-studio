// What both halves of the Tenancy screen (groups, memory pools) work from.
import type { ReactNode } from 'react'
import type { Group, Pool } from '@/api/types'
import { Button, CopyButton, Modal, Banner } from '@/components/ui'
import type { Notice } from '../users/helpers'

/** The registry as loaded, plus the names the pickers and labels draw on. */
export interface Directory {
  groups: Group[]
  pools: Pool[]
  /** Known profile ids, offered as suggestions; any uuid can still be typed. */
  userIDs: string[]
  /** Configured (non-spawn) agent names. */
  agents: string[]
  /** A tenant uuid with its display name when one is known. */
  who: (id: string | undefined) => string
}

export interface PaneProps {
  dir: Directory
  selected: string
  onSelect: (id: string) => void
  /** Bumped by a full reload, so an open detail refetches with the lists. */
  rev: number
  /** Reloads the lists and whatever detail is open. */
  reloadAll: () => void
  notify: (notice: Notice | null) => void
}

/** A tenant (or any) uuid: readable label, monospace, with the full id one click away. */
export function Ident({ label, id, what }: { label: string; id: string; what: string }) {
  return (
    <span className="flex min-w-0 items-center gap-1">
      <span className="truncate font-mono text-xs" title={id}>
        {label}
      </span>
      {id !== '' && <CopyButton text={id} what={what} />}
    </span>
  )
}

/** A form in a modal with Cancel and one primary action. */
export function FormModal({
  title,
  subtitle,
  okLabel,
  busy,
  error,
  onSubmit,
  onClose,
  children,
}: {
  title: string
  subtitle: string
  okLabel: string
  busy: boolean
  error: string
  onSubmit: () => void
  onClose: () => void
  children: ReactNode
}) {
  return (
    <Modal
      open
      onClose={onClose}
      dismissable={!busy}
      width={580}
      title={title}
      subtitle={subtitle}
      footer={
        <>
          <Button onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button variant="primary" className="min-w-28" loading={busy} onClick={onSubmit}>
            {okLabel}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-4"
        onSubmit={(e) => {
          e.preventDefault()
          if (!busy) onSubmit()
        }}
      >
        {children}
        <Banner tone="danger">{error}</Banner>
        {/* Lets Enter in a field submit the form. */}
        <button type="submit" hidden />
      </form>
    </Modal>
  )
}
