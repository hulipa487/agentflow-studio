// Connection: which runtime this tab talks to, how, what this browser has
// saved about it, and the other saved connections.
import { ArrowLeftRight, Lock, Plus, Trash2 } from 'lucide-react'
import { transportOf, type Client } from '@/api/client'
import { Button, Chip, CopyButton, Dot, IconButton, KV, Section, StatusChip } from '@/components/ui'
import { humanDuration, orDash, shortID } from '@/lib/format'
import { tokenFor, useConnections, type Profile } from '@/state/connections'
import { useClient, useSession } from '@/state/session'
import { confirm } from '@/state/ui'
import { Fact, FieldLabel } from './parts'

type TokenState = 'stored' | 'tab' | 'none'

function tokenState(p: Profile | undefined): TokenState {
  if (!p) return 'none'
  if (p.token) return 'stored'
  return tokenFor(p) ? 'tab' : 'none'
}

function transportLabel(url: string, viaProxy: boolean): string {
  const kind = transportOf(url)
  if (kind === 'unix') return 'Local socket · via local proxy'
  if (kind === 'npipe') return 'Named pipe · via local proxy'
  return viaProxy ? 'TCP · via local proxy' : 'TCP · direct from this browser'
}

function transportNote(client: Client): string {
  if (client.local) return 'A browser cannot open a socket or pipe, so Studio’s local proxy on this origin does and forwards each request.'
  if (client.viaProxy) return 'Requests go to Studio’s local proxy on this origin, which forwards them to the runtime. No CORS entry is needed.'
  return 'The browser calls the runtime itself, so the runtime must allow this origin (below).'
}

const TOKEN_NOTE: Record<TokenState, string> = {
  stored:
    'Remembered in this browser’s localStorage, in plain text. Anyone who can use this browser profile can read it. Lock forgets it.',
  tab: 'Kept for this tab only (sessionStorage, in plain text). A reload stays signed in; closing the tab forgets it.',
  none: 'Not saved in this browser: it is held in memory, and a reload asks for it again.',
}

function TokenChip({ state }: { state: TokenState }) {
  if (state === 'stored') return <Chip tone="warning">token in localStorage</Chip>
  if (state === 'tab') return <Chip tone="accent">token for this tab</Chip>
  return <Chip>no token saved</Chip>
}

/** Leaves this runtime for another saved one, exactly as the shell's connection menu does. */
async function switchTo(name: string) {
  useConnections.getState().select(name)
  useSession.getState().disconnect()
  // A profile with a token reconnects at once; without one the welcome
  // screen opens prefilled with its address.
  await useSession.getState().restore()
}

async function forget(p: Profile, isActive: boolean) {
  const ok = await confirm({
    title: 'Forget connection',
    message: isActive
      ? `Forget ${p.name}? Its address and token are removed from this browser and you return to the welcome screen.`
      : `Forget ${p.name}? Its address and token are removed from this browser.`,
    confirmLabel: 'Forget',
    danger: true,
  })
  if (!ok) return
  useConnections.getState().remove(p.name)
  if (isActive) useSession.getState().disconnect()
}

export function ConnectionSection() {
  const client = useClient()
  const online = useSession((s) => s.online)
  const rt = useSession((s) => s.runtime)
  const profiles = useConnections((s) => s.profiles)
  const active = useConnections((s) => s.active)
  const current = profiles.find((p) => p.name === active)
  const token = tokenState(current)
  const direct = !client.viaProxy
  const corsLines = `runtime:\n  admin:\n    cors_origins: ["${location.origin}"]`

  return (
    <Section
      title="Connection"
      subtitle="The AgentFlow runtime admin API this Studio window is connected to."
      action={online ? <StatusChip tone="success">connected</StatusChip> : <StatusChip tone="danger" pulse>offline</StatusChip>}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <Fact label="Endpoint">
            <span className="truncate font-mono text-xs" title={client.displayURL}>
              {client.displayURL}
            </span>
            <CopyButton text={client.displayURL} what="endpoint" />
          </Fact>
          <Fact label="Transport" hint={transportNote(client)}>
            {transportLabel(client.displayURL, client.viaProxy)}
          </Fact>
          <Fact label="Admin token" hint={TOKEN_NOTE[token]}>
            <TokenChip state={token} />
          </Fact>
        </div>

        {rt && (
          <div className="grid grid-cols-2 gap-x-6 gap-y-2 rounded-ctl border border-line bg-surface2 px-3 py-2.5 md:grid-cols-4">
            <KV label="Instance" value={<span title={rt.instance}>{orDash(shortID(rt.instance))}</span>} copy={rt.instance || undefined} />
            <KV label="Region" value={orDash(rt.region)} />
            <KV label="Zone" value={orDash(rt.zone)} />
            <KV label="Topology" value={orDash(rt.topology)} />
            <KV label="Version" value={orDash(rt.version)} />
            <KV label="Uptime" value={humanDuration(rt.uptime_s)} />
            <KV label="Config epoch" value={orDash(rt.config_epoch)} />
            <KV label="Config" value={<span title={rt.config_path}>{orDash(rt.config_path)}</span>} copy={rt.config_path || undefined} />
          </div>
        )}

        {direct && (
          <div>
            <FieldLabel>Allowed origin</FieldLabel>
            <p className="mb-1.5 text-xs leading-snug text-muted">
              A direct connection works only while the runtime lists this page’s origin under <span className="font-mono">runtime.admin.cors_origins</span>.
              Keep this in its config (a change applies at the next restart):
            </p>
            <div className="flex items-start gap-2 rounded-ctl border border-line bg-bg py-2 pl-3 pr-2">
              <pre className="min-w-0 flex-1 overflow-x-auto font-mono text-xs leading-relaxed text-text">{corsLines}</pre>
              <CopyButton text={corsLines} what="cors_origins" />
            </div>
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          <Button icon={Lock} onClick={() => useSession.getState().lock()} title="Forget the token and return to the welcome screen">
            Lock
          </Button>
          <Button
            icon={Plus}
            onClick={() => {
              // Keep the saved profiles; only the active mirror is cleared so
              // the welcome screen starts blank.
              useConnections.getState().clearActive()
              useSession.getState().disconnect()
            }}
          >
            Use another connection…
          </Button>
          <span className="text-xs text-muted">Lock forgets the token and returns to the welcome screen; the address stays saved.</span>
        </div>

        <div>
          <FieldLabel>Saved connections</FieldLabel>
          {profiles.length === 0 ? (
            <div className="rounded-ctl border border-line px-3 py-3 text-xs text-muted">No saved connections yet.</div>
          ) : (
            <div className="overflow-hidden rounded-ctl border border-line">
              {profiles.map((p) => {
                const isActive = p.name === active
                return (
                  <div key={p.name} className="flex min-h-10 items-center gap-2.5 border-b border-line/70 px-3 py-1.5 last:border-b-0">
                    <Dot tone={isActive ? 'success' : 'neutral'} />
                    <div className="min-w-0 flex-1">
                      <div className="flex min-w-0 items-center gap-2">
                        <span className={'truncate text-base ' + (isActive ? 'font-semibold' : '')}>{p.name}</span>
                        {isActive && <Chip tone="success">active</Chip>}
                      </div>
                      <div className="truncate font-mono text-xs text-muted" title={p.url}>
                        {p.url}
                      </div>
                    </div>
                    <Chip>{transportLabel(p.url, p.viaProxy)}</Chip>
                    <TokenChip state={tokenState(p)} />
                    {!isActive && (
                      <Button
                        size="sm"
                        icon={ArrowLeftRight}
                        onClick={() => void switchTo(p.name)}
                        title={tokenState(p) === 'none' ? 'Opens the welcome screen to enter its token' : 'Reconnect with this profile'}
                      >
                        Switch
                      </Button>
                    )}
                    <IconButton icon={Trash2} label={`Forget ${p.name}`} tone="danger" onClick={() => void forget(p, isActive)} />
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </Section>
  )
}
