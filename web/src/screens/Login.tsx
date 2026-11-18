// The welcome screen: pick a transport, name the runtime, give the token.
// A failure points at the field to fix, and nothing is saved until a
// connection has actually worked.
import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowRight, Eye, EyeOff, Moon, Sun, Trash2, X } from 'lucide-react'
import { ApiError, errorMessage, normalizeURL, probeProxy, transportOf, type Transport } from '@/api/client'
import { describeURL } from '@/api/describe'
import { Banner, Button, Checkbox, Chip, Field, IconButton, Input, Logo, Segmented, Spinner, cx } from '@/components/ui'
import { APP_VERSION } from '@/components/shell/Shell'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { tokenFor, useConnections } from '@/state/connections'
import { useSession } from '@/state/session'

const EXAMPLES: Record<Transport, string> = {
  http: 'http://127.0.0.1:8080',
  unix: 'unix:///run/agentflow/admin.sock',
  npipe: 'npipe://agentflow-admin',
}

const TOKEN_HINT =
  'ADMIN_TOKEN, a named operator token (runtime.admin.operators), or the per-boot token AgentFlow prints at startup.'

export default function Login() {
  const profiles = useConnections((s) => s.profiles)
  const active = useConnections((s) => s.active)
  const connect = useSession((s) => s.connect)
  const mode = useResolvedMode()
  const toggleTheme = useAppearance((s) => s.toggle)

  const initial = profiles.find((p) => p.name === active)
  const [url, setURL] = useState(initial?.url ?? EXAMPLES.http)
  const [token, setToken] = useState(tokenFor(initial))
  const [remember, setRemember] = useState(initial ? !initial.forgetToken : true)
  const [showToken, setShowToken] = useState(false)
  const [proxy, setProxy] = useState({ available: false, platform: '', probed: false })
  const [viaProxy, setViaProxy] = useState(initial?.viaProxy ?? true)
  const [urlError, setURLError] = useState('')
  const [tokenError, setTokenError] = useState('')
  const [message, setMessage] = useState<{ text: string; tone: 'danger' | 'neutral' } | null>(() => {
    // An automatic reconnect that failed says why, once.
    const text = useSession.getState().restoreError
    return text ? { text, tone: 'danger' } : null
  })
  const [step, setStep] = useState('')
  const abort = useRef<AbortController | null>(null)
  const urlInput = useRef<HTMLInputElement>(null)
  const tokenInput = useRef<HTMLInputElement>(null)
  const busy = step !== ''

  useEffect(() => {
    let live = true
    probeProxy().then((p) => {
      if (!live) return
      setProxy({ ...p, probed: true })
      // Without the proxy the only route is direct.
      if (!p.available) setViaProxy(false)
      else if (!initial) setViaProxy(true)
    })
    return () => {
      live = false
      abort.current?.abort()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    // Start in the field that still needs typing.
    if (url.trim() !== '' && token.trim() === '') tokenInput.current?.focus()
    else urlInput.current?.focus()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const kind = transportOf(url)
  const described = useMemo(() => describeURL(url, proxy), [url, proxy])
  const forcedProxy = kind !== 'http'
  const usingProxy = forcedProxy || (viaProxy && proxy.available)

  const pickTransport = (t: Transport) => {
    if (transportOf(url) !== t) setURL(EXAMPLES[t])
    setURLError('')
    setMessage(null)
    urlInput.current?.focus()
  }

  /** Marks the field a failure points at and returns the message to show. */
  const explain = (err: unknown): string => {
    let text = errorMessage(err)
    if (err instanceof ApiError) {
      if (err.status === 401) {
        setTokenError(
          'The runtime rejected this token. Check ADMIN_TOKEN, or copy the token AgentFlow printed at boot (it changes every start unless ADMIN_TOKEN is set).',
        )
        text = 'Authentication failed: ' + err.message
        tokenInput.current?.focus()
      } else if (err.status === 0 || err.status === 502) {
        setURLError(
          'Nothing answered here. Is AgentFlow running, and is this its address (gateway.listen, or runtime.admin.socket with tcp: false)?',
        )
        urlInput.current?.focus()
      } else if (err.status === 403) {
        setURLError(
          'This runtime answers its admin API only on its own machine: its token is minted per boot. Set ADMIN_TOKEN (or runtime.admin.operators) on the runtime to connect from here.',
        )
        urlInput.current?.focus()
      } else if (err.status === 503) {
        text += '\n\nThe Studio API is off on this runtime: set runtime.studio.enabled: true and restart AgentFlow.'
      }
    }
    return text
  }

  const fail = (err: unknown) => setMessage({ text: explain(err), tone: 'danger' })

  // A failed automatic reconnect gets the same field hints as a failed
  // Connect; its own message, which names the connection, is already shown.
  // Declared after the focus effect above so the hinted field keeps focus.
  useEffect(() => {
    const cause = useSession.getState().restoreCause
    if (cause !== undefined) explain(cause)
    useSession.setState({ restoreError: '', restoreCause: undefined })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const submit = async () => {
    if (busy) {
      abort.current?.abort()
      return
    }
    setMessage(null)
    setURLError('')
    setTokenError('')
    if (url.trim() === '') {
      setURLError('An instance URL is required.')
      urlInput.current?.focus()
      return
    }
    if (!described.ok) {
      setURLError(described.text)
      urlInput.current?.focus()
      return
    }
    if (token.trim() === '') {
      setTokenError('The admin token is required.')
      tokenInput.current?.focus()
      return
    }
    const ctl = new AbortController()
    abort.current = ctl
    let timedOut = false
    const timer = setTimeout(() => {
      timedOut = true
      ctl.abort()
    }, 20000)
    setStep(`contacting ${normalizeURL(url)}…`)
    try {
      await connect(url, token, { remember, viaProxy: usingProxy, signal: ctl.signal, onStep: setStep })
    } catch (err) {
      if (ctl.signal.aborted) {
        if (timedOut) {
          setURLError('No answer within 20 seconds. Check the address and that nothing blocks the port.')
          setMessage({ text: `Timed out contacting ${normalizeURL(url)}.`, tone: 'danger' })
        } else {
          setMessage({ text: 'Connection cancelled.', tone: 'neutral' })
        }
      } else {
        fail(err)
      }
    } finally {
      clearTimeout(timer)
      abort.current = null
      setStep('')
    }
  }

  return (
    <div className="relative flex h-full flex-col items-center overflow-y-auto px-4 py-10">
      <div className="my-auto flex w-full max-w-[480px] flex-col items-center">
        <Logo size={68} />
        <h1 className="mt-4 text-xl font-semibold tracking-[-0.01em]">
          AgentFlow <span className="flag-text">Studio</span>
        </h1>
        <p className="mt-1 text-base text-muted">Build, run and trace agents on your own runtime.</p>
        <div className="flag-stripe mt-4 h-[3px] w-44 rounded-full" />

        <form
          className="mt-6 flex w-full flex-col gap-3.5 rounded-card border border-line bg-surface p-5"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <div>
            <h2 className="text-md font-semibold">Connect to a runtime</h2>
            <p className="mt-0.5 text-sm text-muted">
              Studio talks to AgentFlow’s admin API with its bearer token. Nothing is stored on a server.
            </p>
          </div>

          {profiles.length > 0 && (
            <Field label="Saved connections">
              <div className="flex flex-col gap-1">
                {profiles.map((p) => (
                  <div key={p.name} className="group flex items-center gap-1">
                    <button
                      type="button"
                      disabled={busy}
                      onClick={() => {
                        setURL(p.url)
                        setToken(tokenFor(p))
                        setRemember(!p.forgetToken)
                        setViaProxy(p.viaProxy && proxy.available)
                        setURLError('')
                        setTokenError('')
                        setMessage(null)
                      }}
                      className={cx(
                        'flex h-[30px] min-w-0 flex-1 items-center gap-2 rounded-ctl border px-2.5 text-left hover:bg-hover',
                        normalizeURL(url) === p.url ? 'border-accent/60 bg-accent-dim/40' : 'border-line bg-bg',
                      )}
                    >
                      <span className="truncate text-base font-medium">{p.name}</span>
                      <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{p.url}</span>
                      {tokenFor(p) ? <Chip tone="success">token saved</Chip> : <Chip>no token</Chip>}
                    </button>
                    <IconButton icon={Trash2} label={`Forget ${p.name}`} tone="danger" onClick={() => useConnections.getState().remove(p.name)} />
                  </div>
                ))}
              </div>
            </Field>
          )}

          <Field label="Transport">
            <Segmented<Transport>
              className="self-start"
              value={kind}
              onChange={pickTransport}
              options={[
                { value: 'http', label: 'HTTP' },
                { value: 'unix', label: 'Unix socket', disabled: busy || (proxy.probed && !proxy.available), title: proxy.available ? undefined : 'Needs the local proxy' },
                {
                  value: 'npipe',
                  label: 'Named pipe',
                  disabled: busy || (proxy.probed && (!proxy.available || proxy.platform !== 'win32')),
                  title: proxy.platform !== 'win32' ? 'Named pipes are Windows only' : undefined,
                },
              ]}
            />
          </Field>

          <Field label="Instance URL" error={urlError || (!described.ok ? described.text : '')} hint={described.text}>
            <Input
              ref={urlInput}
              mono
              value={url}
              disabled={busy}
              invalid={urlError !== '' || !described.ok}
              placeholder={EXAMPLES[kind]}
              onChange={(e) => {
                setURL(e.target.value)
                setURLError('')
                setMessage(null)
              }}
              onKeyDown={(e) => {
                // Enter moves on to the token while there is none to submit.
                if (e.key === 'Enter' && token.trim() === '') {
                  e.preventDefault()
                  tokenInput.current?.focus()
                }
              }}
            />
          </Field>

          <Field label="Admin token" error={tokenError} hint={TOKEN_HINT}>
            <div className="relative">
              <Input
                ref={tokenInput}
                mono
                type={showToken ? 'text' : 'password'}
                value={token}
                disabled={busy}
                invalid={tokenError !== ''}
                placeholder="ADMIN_TOKEN or an operator token"
                autoComplete="off"
                className="pr-9"
                onChange={(e) => {
                  setToken(e.target.value)
                  setTokenError('')
                  setMessage(null)
                }}
              />
              <IconButton
                icon={showToken ? Eye : EyeOff}
                label={showToken ? 'Hide the token' : 'Show the token'}
                size="sm"
                className="absolute right-1.5 top-1/2 -translate-y-1/2"
                onClick={() => setShowToken((v) => !v)}
              />
            </div>
          </Field>

          <div className="flex flex-col gap-2.5 rounded-ctl border border-line bg-bg p-3">
            <Checkbox
              checked={usingProxy}
              disabled={busy || forcedProxy || !proxy.available}
              onChange={setViaProxy}
              label="Route through Studio’s local proxy"
              hint={
                !proxy.probed
                  ? 'Checking for the local proxy…'
                  : !proxy.available
                    ? `Not available on this origin, so the browser calls the runtime directly: add "${location.origin}" to runtime.admin.cors_origins.`
                    : forcedProxy
                      ? 'Required for sockets and pipes: a browser cannot open them itself.'
                      : usingProxy
                        ? 'No CORS setup needed: the browser talks to this origin, which forwards to the runtime.'
                        : `Direct from the browser: the runtime must list "${location.origin}" in runtime.admin.cors_origins.`
              }
            />
            <Checkbox
              checked={remember}
              disabled={busy}
              onChange={setRemember}
              label="Remember the token in this browser"
              hint={
                remember
                  ? 'Stored in plain text in this browser profile’s localStorage. Lock forgets it.'
                  : 'Kept for this tab only; closing the tab forgets it.'
              }
            />
          </div>

          {message && (
            <Banner tone={message.tone} onClose={() => setMessage(null)}>
              {message.text}
            </Banner>
          )}

          <Button type="submit" variant={busy ? 'default' : 'primary'} icon={busy ? X : ArrowRight} className="h-9 w-full">
            {busy ? 'Cancel' : 'Connect'}
          </Button>
          {busy && (
            <div className="flex items-center justify-center gap-2 text-xs text-muted">
              <Spinner className="size-3.5" /> {step}
            </div>
          )}
        </form>

        <div className="mt-4 flex items-center gap-2 text-xs text-faint">
          <span>Studio {APP_VERSION} ·</span>
          <button type="button" onClick={toggleTheme} className="inline-flex items-center gap-1.5 rounded px-1.5 py-1 font-medium text-muted hover:bg-hover hover:text-text">
            {mode === 'light' ? <Moon className="size-3.5" /> : <Sun className="size-3.5" />}
            {mode === 'light' ? 'Dark mode' : 'Light mode'}
          </button>
        </div>
      </div>
    </div>
  )
}
