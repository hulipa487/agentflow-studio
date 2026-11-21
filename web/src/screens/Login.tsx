// The welcome screen: pick a transport, name the runtime, give the token.
// A failure points at the field to fix, and nothing is saved until a
// connection has actually worked.
import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, BookOpen, Eye, EyeOff, Moon, Sun, Trash2, X } from 'lucide-react'
import { ApiError, errorMessage, normalizeURL, probeProxy, transportOf, type Transport } from '@/api/client'
import { describeURL } from '@/api/describe'
import { Banner, BrandBanner, Button, Checkbox, Chip, Field, IconButton, Input, Segmented, Spinner, cx } from '@/components/ui'
import { APP_VERSION } from '@/components/shell/Shell'
import { t, useLang, type Params } from '@/i18n'
import { useAppearance, useResolvedMode } from '@/state/appearance'
import { tokenFor, useConnections } from '@/state/connections'
import { useSession } from '@/state/session'

const EXAMPLES: Record<Transport, string> = {
  http: 'http://127.0.0.1:8080',
  unix: 'unix:///run/agentflow/admin.sock',
  npipe: 'npipe://agentflow-admin',
}

// A message on this screen is kept as its key (or as the runtime's own text)
// and translated when it is drawn, so switching language while one is shown
// changes it too.
type Msg = { key: string; params?: Params } | { text: string } | { described: true }

export default function Login() {
  const navigate = useNavigate()
  const profiles = useConnections((s) => s.profiles)
  const active = useConnections((s) => s.active)
  const connect = useSession((s) => s.connect)
  const mode = useResolvedMode()
  const toggleTheme = useAppearance((s) => s.toggle)
  const lang = useLang()

  const initial = profiles.find((p) => p.name === active)
  const [url, setURL] = useState(initial?.url ?? EXAMPLES.http)
  const [token, setToken] = useState(tokenFor(initial))
  const [remember, setRemember] = useState(initial ? !initial.forgetToken : true)
  const [showToken, setShowToken] = useState(false)
  const [proxy, setProxy] = useState({ available: false, platform: '', probed: false })
  const [viaProxy, setViaProxy] = useState(initial?.viaProxy ?? true)
  const [urlError, setURLError] = useState<Msg | null>(null)
  const [tokenError, setTokenError] = useState<Msg | null>(null)
  const [message, setMessage] = useState<{ msg: Msg; tone: 'danger' | 'neutral' } | null>(() => {
    // An automatic reconnect that failed says why, once.
    const text = useSession.getState().restoreError
    return text ? { msg: { text }, tone: 'danger' } : null
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
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'F1') {
        e.preventDefault()
        navigate('/docs')
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate])

  useEffect(() => {
    // Start in the field that still needs typing.
    if (url.trim() !== '' && token.trim() === '') tokenInput.current?.focus()
    else urlInput.current?.focus()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const kind = transportOf(url)
  const described = useMemo(() => describeURL(url, proxy), [url, proxy, lang])
  const forcedProxy = kind !== 'http'
  const usingProxy = forcedProxy || (viaProxy && proxy.available)
  const say = (m: Msg | null): string => (!m ? '' : 'key' in m ? t(m.key, m.params) : 'text' in m ? m.text : described.text)

  const pickTransport = (next: Transport) => {
    if (transportOf(url) !== next) setURL(EXAMPLES[next])
    setURLError(null)
    setMessage(null)
    urlInput.current?.focus()
  }

  /** Marks the field a failure points at and returns the message to show. */
  const explain = (err: unknown): Msg => {
    const text = errorMessage(err)
    if (err instanceof ApiError) {
      if (err.status === 401) {
        setTokenError({ key: 'login.tokenRejected' })
        tokenInput.current?.focus()
        return { key: 'login.authFailed', params: { message: err.message } }
      } else if (err.status === 0 || err.status === 502) {
        setURLError({ key: 'login.nothingAnswered' })
        urlInput.current?.focus()
      } else if (err.status === 403) {
        setURLError({ key: 'login.localOnly' })
        urlInput.current?.focus()
      } else if (err.status === 503) {
        return { key: 'login.studioOff', params: { message: text } }
      }
    }
    return { text }
  }

  const fail = (err: unknown) => setMessage({ msg: explain(err), tone: 'danger' })

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
    setURLError(null)
    setTokenError(null)
    if (url.trim() === '') {
      setURLError({ key: 'login.urlRequired' })
      urlInput.current?.focus()
      return
    }
    if (!described.ok) {
      setURLError({ described: true })
      urlInput.current?.focus()
      return
    }
    if (token.trim() === '') {
      setTokenError({ key: 'login.tokenRequired' })
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
    setStep(t('connection.contacting', { url: normalizeURL(url) }))
    try {
      await connect(url, token, { remember, viaProxy: usingProxy, signal: ctl.signal, onStep: setStep })
    } catch (err) {
      if (ctl.signal.aborted) {
        if (timedOut) {
          setURLError({ key: 'login.noAnswer' })
          setMessage({ msg: { key: 'login.timedOut', params: { url: normalizeURL(url) } }, tone: 'danger' })
        } else {
          setMessage({ msg: { key: 'login.cancelled' }, tone: 'neutral' })
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
        <BrandBanner height={72} />
        {/* The banner carries the AgentFlow name; this line names the app. */}
        <h1 className="mt-1 text-xl font-semibold tracking-[-0.01em]">Studio</h1>
        <p className="mt-1 text-base text-muted">{t('login.tagline')}</p>
        <div className="brand-rule mt-4 h-[3px] w-44 rounded-full" />

        <form
          className="mt-6 flex w-full flex-col gap-3.5 rounded-card border border-line bg-surface p-5"
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
        >
          <div>
            <h2 className="text-md font-semibold">{t('login.connectTitle')}</h2>
            <p className="mt-0.5 text-sm text-muted">{t('login.introWeb')}</p>
          </div>

          {profiles.length > 0 && (
            <Field label={t('login.savedConnections')}>
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
                        setURLError(null)
                        setTokenError(null)
                        setMessage(null)
                      }}
                      className={cx(
                        'flex h-[30px] min-w-0 flex-1 items-center gap-2 rounded-ctl border px-2.5 text-left hover:bg-hover',
                        normalizeURL(url) === p.url ? 'border-accent/60 bg-accent-dim/40' : 'border-line bg-bg',
                      )}
                    >
                      <span className="truncate text-base font-medium">{p.name}</span>
                      <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted">{p.url}</span>
                      {tokenFor(p) ? <Chip tone="success">{t('login.tokenSaved')}</Chip> : <Chip>{t('login.noToken')}</Chip>}
                    </button>
                    <IconButton icon={Trash2} label={t('login.forget', { name: p.name })} tone="danger" onClick={() => useConnections.getState().remove(p.name)} />
                  </div>
                ))}
              </div>
            </Field>
          )}

          <Field label={t('login.transport')}>
            <Segmented<Transport>
              className="self-start"
              value={kind}
              onChange={pickTransport}
              options={[
                { value: 'http', label: t('login.transportHttp') },
                { value: 'unix', label: t('login.transportUnix'), disabled: busy || (proxy.probed && !proxy.available), title: proxy.available ? undefined : t('login.needsProxy') },
                {
                  value: 'npipe',
                  label: t('login.transportPipe'),
                  disabled: busy || (proxy.probed && (!proxy.available || proxy.platform !== 'win32')),
                  title: proxy.platform !== 'win32' ? t('login.pipesWindowsOnly') : undefined,
                },
              ]}
            />
          </Field>

          <Field label={t('login.instanceURL')} error={say(urlError) || (!described.ok ? described.text : '')} hint={described.text}>
            <Input
              ref={urlInput}
              mono
              value={url}
              disabled={busy}
              invalid={urlError !== null || !described.ok}
              placeholder={EXAMPLES[kind]}
              onChange={(e) => {
                setURL(e.target.value)
                setURLError(null)
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

          <Field label={t('login.adminToken')} error={say(tokenError)} hint={t('login.tokenHint')}>
            <div className="relative">
              <Input
                ref={tokenInput}
                mono
                type={showToken ? 'text' : 'password'}
                value={token}
                disabled={busy}
                invalid={tokenError !== null}
                placeholder={t('login.tokenPlaceholder')}
                autoComplete="off"
                className="pr-9"
                onChange={(e) => {
                  setToken(e.target.value)
                  setTokenError(null)
                  setMessage(null)
                }}
              />
              <IconButton
                icon={showToken ? Eye : EyeOff}
                label={showToken ? t('login.hideToken') : t('login.showToken')}
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
              label={t('login.viaProxy')}
              hint={
                !proxy.probed
                  ? t('login.proxyChecking')
                  : !proxy.available
                    ? t('login.proxyUnavailable', { origin: location.origin })
                    : forcedProxy
                      ? t('login.proxyRequired')
                      : usingProxy
                        ? t('login.proxyOn')
                        : t('login.proxyOff', { origin: location.origin })
              }
            />
            <Checkbox
              checked={remember}
              disabled={busy}
              onChange={setRemember}
              label={t('login.rememberBrowser')}
              hint={
                remember ? t('login.rememberBrowserOn') : t('login.rememberBrowserOff')
              }
            />
          </div>

          {message && (
            <Banner tone={message.tone} onClose={() => setMessage(null)}>
              {say(message.msg)}
            </Banner>
          )}

          <Button type="submit" variant={busy ? 'default' : 'primary'} icon={busy ? X : ArrowRight} className="h-9 w-full">
            {busy ? t('common.cancel') : t('login.connect')}
          </Button>
          {busy && (
            <div className="flex items-center justify-center gap-2 text-xs text-muted">
              <Spinner className="size-3.5" /> {step}
            </div>
          )}
        </form>

        <div className="mt-4 flex flex-wrap items-center justify-center gap-2 text-xs text-faint">
          <span>{t('login.version', { version: APP_VERSION })}</span>
          <button type="button" onClick={() => navigate('/docs')} className="inline-flex items-center gap-1.5 rounded px-1.5 py-1 font-medium text-muted hover:bg-hover hover:text-text">
            <BookOpen className="size-3.5" />
            {t('login.browseDocs')}
          </button>
          <button type="button" onClick={toggleTheme} className="inline-flex items-center gap-1.5 rounded px-1.5 py-1 font-medium text-muted hover:bg-hover hover:text-text">
            {mode === 'light' ? <Moon className="size-3.5" /> : <Sun className="size-3.5" />}
            {mode === 'light' ? t('login.darkMode') : t('login.lightMode')}
          </button>
        </div>
      </div>
    </div>
  )
}
