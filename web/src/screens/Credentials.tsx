// Credentials: the encrypted store the runtime resolves secrets from at
// request time, one bucket at a time (engine-wide or one user's). Values are
// write-only: this screen can replace or revoke a secret, never read one, so
// nothing here can reveal a stored value.
import { useState } from 'react'
import { Pencil, Plus, RotateCw, Trash2, User } from 'lucide-react'
import { ApiError, errorMessage } from '@/api/client'
import type { Credential, CredentialList } from '@/api/types'
import { AsyncView, Banner, Button, Chip, EmptyState, IconButton, Page, Panel, Select, Table, type Column } from '@/components/ui'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { localDateTime, plural, relTime } from '@/lib/format'
import { useClient } from '@/state/session'
import { confirm, toast } from '@/state/ui'
import { BucketDialog, CredentialEditor, useMounted } from './credentials/dialogs'
import { DISABLED_NOTE, bucketLabel, bucketOptions, bucketPhrase, disabledList, injectionShape } from './credentials/helpers'

type Dialog = { kind: 'edit'; existing: Credential | null } | { kind: 'bucket' }

function When({ unix }: { unix: number }) {
  if (!unix || unix <= 0) return <span className="text-faint">—</span>
  return <span title={localDateTime(new Date(unix * 1000))}>{relTime(unix)}</span>
}

export default function Credentials() {
  const client = useClient()
  const mounted = useMounted()
  // '' is the engine-wide bucket that cred:<service> resolves against.
  const [bucket, setBucket] = useState('')
  const { data, error, loading, refreshing, reload, setData } = useAsync<CredentialList>(
    async (signal) => {
      try {
        return await client.credentials(bucket, signal)
      } catch (err) {
        // A runtime without the store is a state to explain, not a failure.
        if (err instanceof ApiError && err.status === 503) return disabledList(DISABLED_NOTE)
        if (err instanceof ApiError && err.status === 404) {
          throw new ApiError(404, 'This runtime does not expose the Studio credentials API yet; rebuild and restart AgentFlow, then reopen Credentials.')
        }
        throw err
      }
    },
    [client, bucket],
  )
  const [dialog, setDialog] = useState<Dialog | null>(null)
  const [actionError, setActionError] = useState('')
  const [deleting, setDeleting] = useState('')

  // While another bucket loads, the previous bucket's rows are still in
  // `data`. They are not shown: an edit or delete must never land on a
  // bucket other than the one named in the header.
  const list = data && (data.bucket === bucket || !data.enabled) ? data : undefined
  const enabled = list?.enabled ?? true
  const rows = list?.credentials ?? []

  const refresh = () => {
    setActionError('')
    reload()
  }
  useScreenActions({ refresh })

  const selectBucket = (user: string) => {
    setActionError('')
    setBucket(user)
  }

  const remove = async (c: Credential) => {
    if (!list) return
    const user = list.bucket
    const ok = await confirm({
      title: 'Delete credential',
      message: `Delete credential "${c.service}" from ${user === '' ? 'the engine-wide bucket' : 'user ' + user}?\n\nAny loop or config reference that resolves it will fail until a new secret is stored.`,
      confirmLabel: 'Delete',
      danger: true,
    })
    if (!ok || !mounted.current) return
    setDeleting(c.service)
    try {
      const next = await client.deleteCredential(user, c.service)
      if (!mounted.current) return
      setActionError('')
      setData(next)
      toast(`credential ${c.service} deleted`, 'warning')
    } catch (err) {
      if (!mounted.current) return
      // The store went away under the screen: show the disabled state.
      if (err instanceof ApiError && err.status === 503) reload()
      else setActionError(errorMessage(err))
    } finally {
      if (mounted.current) setDeleting('')
    }
  }

  const columns: Column<Credential>[] = [
    {
      key: 'service',
      title: 'Service',
      render: (c) => (
        <span className="font-mono text-xs font-semibold text-text" title={c.service}>
          {c.service}
        </span>
      ),
    },
    { key: 'kind', title: 'Kind', width: 104, render: (c) => <Chip tone="accent">{c.kind || 'api_key'}</Chip> },
    {
      key: 'fingerprint',
      title: 'Fingerprint',
      width: 112,
      render: (c) =>
        c.fingerprint ? (
          <Chip tone="success" title="The last four characters of the secret">
            {c.fingerprint}
          </Chip>
        ) : (
          <span className="font-mono text-[10.5px] text-faint">no fingerprint</span>
        ),
    },
    {
      key: 'injection',
      title: 'Sent as',
      render: (c) => (
        <span className="font-mono text-xs text-muted" title={injectionShape(c)}>
          {injectionShape(c)}
        </span>
      ),
    },
    { key: 'created', title: 'Created', width: 104, render: (c) => <When unix={c.created_at} /> },
    { key: 'updated', title: 'Last changed', width: 112, render: (c) => <When unix={c.updated_at} /> },
    {
      key: 'actions',
      title: '',
      width: 84,
      align: 'right',
      render: (c) => (
        <span className="inline-flex items-center justify-end">
          <IconButton icon={Pencil} label={`Edit ${c.service}`} onClick={() => setDialog({ kind: 'edit', existing: c })} />
          <IconButton
            icon={Trash2}
            tone="danger"
            label={`Delete ${c.service}`}
            disabled={deleting !== ''}
            loading={deleting === c.service}
            onClick={() => void remove(c)}
          />
        </span>
      ),
    },
  ]

  return (
    <Page
      title="Credentials"
      subtitle="Encrypted secrets resolved by the runtime at request time. Values are write-only: Studio can replace a secret but never read it back."
      actions={
        <>
          <Select
            mono
            className="w-64"
            aria-label="Bucket"
            value={bucket}
            onChange={selectBucket}
            options={bucketOptions(data?.users, bucket).map((u) => ({ value: u, label: bucketLabel(u) }))}
            disabled={!enabled}
          />
          {list && enabled && (
            <Button icon={User} onClick={() => setDialog({ kind: 'bucket' })}>
              User bucket…
            </Button>
          )}
          {list && <span className="font-mono text-[11px] text-faint">{plural(rows.length, 'secret')}</span>}
          <IconButton icon={RotateCw} label="Refresh" loading={refreshing} onClick={refresh} />
          {list && enabled && rows.length > 0 && (
            <Button variant="primary" icon={Plus} onClick={() => setDialog({ kind: 'edit', existing: null })}>
              New credential
            </Button>
          )}
        </>
      }
      meta={
        list &&
        (enabled ? (
          <>
            <span>{bucketPhrase(list.bucket)}</span>
            <span>write-only values</span>
            <span>saves are live immediately</span>
          </>
        ) : (
          <span>credential store disabled</span>
        ))
      }
      banner={
        list && (actionError || (!enabled && list.note)) ? (
          <>
            {actionError && (
              <Banner tone="danger" onClose={() => setActionError('')}>
                {actionError}
              </Banner>
            )}
            {!enabled && list.note && <Banner tone="warning">{list.note}</Banner>}
          </>
        ) : undefined
      }
    >
      <AsyncView loading={loading || (!!data && !list)} error={error} hasData={!!list} retry={refresh} loadingText="Loading credentials…">
        {!list ? null : !enabled ? (
          <EmptyState
            title="Credentials are disabled"
            guidance="Set runtime.credentials.enabled: true and CREDENTIALS_MASTER_KEY, restart AgentFlow, then reopen this screen."
          />
        ) : rows.length === 0 ? (
          <EmptyState
            title="No credentials in this bucket"
            guidance="Store an API key or token here; loops reference it by name and the runtime injects the value at request time."
            actionLabel="Add a credential"
            onAction={() => setDialog({ kind: 'edit', existing: null })}
          />
        ) : (
          <Panel flush className="overflow-hidden">
            <Table columns={columns} rows={rows} rowKey={(c) => c.service} />
          </Panel>
        )}
      </AsyncView>

      {list && enabled && dialog?.kind === 'edit' && (
        <CredentialEditor
          client={client}
          bucket={list.bucket}
          existing={dialog.existing}
          onClose={() => setDialog(null)}
          onSaved={(next, draft) => {
            setDialog(null)
            setActionError('')
            setData(next)
            toast(`credential ${draft.service} saved · live`, 'success')
          }}
        />
      )}
      {dialog?.kind === 'bucket' && (
        <BucketDialog
          client={client}
          onClose={() => setDialog(null)}
          onOpen={(user) => {
            setDialog(null)
            selectBucket(user)
          }}
        />
      )}
    </Page>
  )
}
