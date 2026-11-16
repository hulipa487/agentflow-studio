// Users: the profiles behind the identity layer. The list is on the left; the
// selected profile (handles, quota, model layer, audit trail) lives in the URL.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { RotateCw, Send, User } from 'lucide-react'
import { errorMessage, featureOff } from '@/api/client'
import type { Invite } from '@/api/types'
import { AsyncView, Banner, Button, IconButton, Loading, Page, SearchBox } from '@/components/ui'
import { Split } from '@/components/ui/Split'
import { useAsync, useScreenActions } from '@/lib/hooks'
import { useClient } from '@/state/session'
import { info, toastError } from '@/state/ui'
import { InviteDialog } from './users/dialogs'
import { byText, useAlive, userSummary, userTitle, type Notice } from './users/helpers'
import { PaneNote, RailRow } from './users/rail'
import { UserDetail } from './users/UserDetail'

export default function Users() {
  const client = useClient()
  const navigate = useNavigate()
  const { id } = useParams<{ id: string }>()
  const [params, setParams] = useSearchParams()
  const alive = useAlive()
  const [filter, setFilter] = useState('')
  const [notice, setNotice] = useState<Notice | null>(null)
  const [invite, setInvite] = useState<Invite | null>(null)
  const [inviting, setInviting] = useState(false)

  const list = useAsync(
    async (signal) => {
      const l = await client.users(signal)
      // A list that loads again supersedes whatever the last action reported.
      if (!signal.aborted) setNotice(null)
      return l
    },
    [client],
  )
  const detail = useAsync((signal) => client.user(id ?? '', signal), [client, id], { enabled: !!id })

  const refresh = () => {
    list.reload()
    if (id) detail.reload()
  }
  useScreenActions({ refresh, back: id ? () => navigate('/users') : undefined })

  const users = useMemo(() => [...(list.data?.users ?? [])].sort((a, b) => byText(userTitle(a), userTitle(b))), [list.data])
  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase()
    if (q === '') return users
    return users.filter((u) => `${u.display_name ?? ''} ${u.email ?? ''} ${u.user_id}`.toLowerCase().includes(q))
  }, [users, filter])

  // Mints a single-use registration code and shows it once.
  const inFlight = useRef(false)
  const issueInvite = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    setInviting(true)
    try {
      const inv = await client.issueInvite()
      if (alive.current) setInvite(inv)
    } catch (err) {
      const off = featureOff(err)
      if (off) void info({ title: 'Invites unavailable', message: off })
      else toastError(err)
    } finally {
      inFlight.current = false
      if (alive.current) setInviting(false)
    }
  }, [client, alive])

  // The command palette links here with ?invite=1: act on it once and drop it
  // from the URL, so a reload or the back button does not mint another code.
  const wantInvite = params.has('invite')
  const consumed = useRef(false)
  useEffect(() => {
    if (!wantInvite) {
      consumed.current = false
      return
    }
    if (consumed.current) return
    consumed.current = true
    const next = new URLSearchParams(params)
    next.delete('invite')
    setParams(next, { replace: true })
    void issueInvite()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [wantInvite])

  const total = users.length
  const count = total === 0 ? 'no profiles yet' : shown.length === 0 ? 'no match' : `${shown.length} of ${total}`
  // The detail on screen must be the selected profile's, not the previous one's.
  const current = id && detail.data?.profile?.user_id === id ? detail.data : undefined

  const left = (
    <div className="flex min-h-0 flex-1 flex-col gap-2">
      <SearchBox value={filter} onChange={setFilter} placeholder="Filter by name, email or id" />
      <div className="px-1 font-mono text-[11px] text-faint">{count}</div>
      <div className="-mx-1 flex min-h-0 flex-1 flex-col gap-0.5 overflow-y-auto px-1">
        {total === 0 && <PaneNote>Nobody has registered. Issue an invite, or let users register through /v1/users.</PaneNote>}
        {shown.map((u) => (
          <RailRow
            key={u.user_id}
            icon={User}
            title={userTitle(u)}
            caption={userSummary(u)}
            active={u.user_id === id}
            onClick={() => navigate('/users/' + encodeURIComponent(u.user_id))}
          />
        ))}
      </div>
    </div>
  )

  const right = (
    <div className="min-h-0 flex-1 overflow-y-auto">
      {!id ? (
        total > 0 && <PaneNote lowered>Select a profile to see its handles, usage and audit trail.</PaneNote>
      ) : current ? (
        <UserDetail
          key={id}
          detail={current}
          onNotice={(n) => {
            if (alive.current) setNotice(n)
          }}
          onChanged={(scope) => {
            if (!alive.current) return
            setNotice(null)
            if (scope === 'all') list.reload()
            detail.reload()
          }}
          onDeleted={() => {
            if (!alive.current) return
            navigate('/users', { replace: true })
            list.reload()
          }}
        />
      ) : detail.error && !detail.loading ? (
        <Banner tone="danger">{errorMessage(detail.error)}</Banner>
      ) : (
        <Loading text="Loading profile…" />
      )}
    </div>
  )

  return (
    <Page
      title="Users"
      subtitle="Profiles behind the identity layer: linked channel handles, today's quota, and the per-user model and instruction layer. Requires runtime.identity.enabled."
      scroll={false}
      meta={
        list.data ? (
          <span>
            {total} profiles · quota day {list.data.day} (UTC)
          </span>
        ) : null
      }
      banner={
        notice ? (
          <Banner tone={notice.tone} onClose={() => setNotice(null)}>
            {notice.text}
          </Banner>
        ) : null
      }
      actions={
        <>
          <IconButton icon={RotateCw} label="Refresh" onClick={refresh} />
          <Button variant="primary" icon={Send} loading={inviting} onClick={issueInvite}>
            Invite
          </Button>
        </>
      }
    >
      <AsyncView loading={list.loading} error={list.error} hasData={!!list.data} retry={() => list.reload()} loadingText="Loading…">
        <Split fixed="left" initial={340} min={260} max={560} storageKey="users" left={left} right={right} />
      </AsyncView>
      <InviteDialog invite={invite} onClose={() => setInvite(null)} />
    </Page>
  )
}
