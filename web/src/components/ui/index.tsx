// The component kit. Every screen is built from these, so spacing, radii and
// tones stay consistent; they are the web counterparts of the desktop app's
// building blocks in internal/ui/theme.go and screen.go.
import {
  forwardRef,
  useEffect,
  useId,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  type SelectHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { createPortal } from 'react-dom'
import clsx, { type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'
import { Check, ChevronDown, Copy, Loader2, RotateCw, Search, X, type LucideIcon } from 'lucide-react'
import { featureOff, errorMessage } from '@/api/client'
import { copyText, humanCount } from '@/lib/format'
import { toast, type Tone } from '@/state/ui'

export type { Tone }

// The theme's own radius and shadow names, so a caller's override of them
// merges like any built-in utility.
const merge = extendTailwindMerge({
  extend: { theme: { radius: ['card', 'ctl'], shadow: ['float'] } },
})

/**
 * Joins class names; when two utilities set the same property the later one
 * wins, so a caller's className reliably overrides a component's default.
 */
export function cx(...v: ClassValue[]): string {
  return merge(clsx(v))
}

// ---- tones ------------------------------------------------------------------

const TONE_TEXT: Record<Tone, string> = {
  neutral: 'text-muted',
  accent: 'text-accent',
  success: 'text-success',
  warning: 'text-warning',
  danger: 'text-danger',
}
const TONE_BG: Record<Tone, string> = {
  neutral: 'bg-surface2',
  accent: 'bg-tone-accent',
  success: 'bg-tone-success',
  warning: 'bg-tone-warning',
  danger: 'bg-tone-danger',
}
const TONE_DOT: Record<Tone, string> = {
  neutral: 'bg-muted',
  accent: 'bg-accent',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
}
const TONE_BORDER: Record<Tone, string> = {
  neutral: 'border-line',
  accent: 'border-accent/50',
  success: 'border-success/50',
  warning: 'border-warning/50',
  danger: 'border-danger/50',
}

export const toneText = (t: Tone) => TONE_TEXT[t]
export const toneBg = (t: Tone) => TONE_BG[t]

// ---- buttons ----------------------------------------------------------------

type ButtonVariant = 'primary' | 'default' | 'ghost' | 'danger'

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: 'sm' | 'md'
  icon?: LucideIcon
  loading?: boolean
}

const BTN_VARIANT: Record<ButtonVariant, string> = {
  primary: 'bg-accent text-on-primary font-semibold hover:brightness-110 active:brightness-95 border-transparent',
  default: 'bg-surface2 text-text font-medium hover:bg-hover active:bg-pressed border-line',
  ghost: 'bg-transparent text-muted font-medium hover:bg-hover hover:text-text active:bg-pressed border-transparent',
  danger: 'bg-danger text-on-primary font-semibold hover:brightness-110 active:brightness-95 border-transparent',
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'default', size = 'md', icon: Icon, loading, className, children, disabled, type, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type ?? 'button'}
      disabled={disabled || loading}
      className={cx(
        'inline-flex shrink-0 select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-ctl border transition-[background-color,filter,opacity] duration-100',
        size === 'sm' ? 'h-7 px-2.5 text-xs' : 'h-[30px] px-3 text-[13px]',
        BTN_VARIANT[variant],
        'disabled:opacity-50',
        className,
      )}
      {...rest}
    >
      {loading ? <Loader2 className="size-3.5 animate-spin" /> : Icon ? <Icon className="size-3.5" strokeWidth={2} /> : null}
      {children}
    </button>
  )
})

export interface IconButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  icon: LucideIcon
  /** Tooltip and accessible name. */
  label: string
  tone?: Tone
  size?: 'sm' | 'md'
  loading?: boolean
}

export const IconButton = forwardRef<HTMLButtonElement, IconButtonProps>(function IconButton(
  { icon: Icon, label, tone = 'neutral', size = 'md', loading, className, disabled, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type="button"
      title={label}
      aria-label={label}
      disabled={disabled || loading}
      className={cx(
        'inline-flex shrink-0 items-center justify-center rounded-ctl text-muted transition-colors duration-100 hover:bg-hover active:bg-pressed disabled:opacity-40',
        size === 'sm' ? 'size-6' : 'size-[30px]',
        tone === 'danger' ? 'hover:text-danger' : 'hover:text-text',
        className,
      )}
      {...rest}
    >
      {loading ? <Loader2 className="size-3.5 animate-spin" /> : <Icon className={size === 'sm' ? 'size-3.5' : 'size-4'} strokeWidth={1.9} />}
    </button>
  )
})

export function CopyButton({ text, what = 'value', size = 'sm', className }: { text: string; what?: string; size?: 'sm' | 'md'; className?: string }) {
  const [done, setDone] = useState(false)
  return (
    <IconButton
      icon={done ? Check : Copy}
      label={`Copy ${what}`}
      size={size}
      className={className}
      onClick={async (e) => {
        e.stopPropagation()
        if (await copyText(text)) {
          setDone(true)
          toast(`${what} copied`, 'success')
          setTimeout(() => setDone(false), 1200)
        } else {
          toast('could not reach the clipboard', 'danger')
        }
      }}
    />
  )
}

// ---- form controls ----------------------------------------------------------

const CONTROL =
  'h-[30px] w-full rounded-ctl border border-line bg-bg px-2.5 text-[13px] text-text placeholder:text-faint transition-colors duration-100 ' +
  'hover:border-faint focus:border-accent focus:outline-none focus:ring-2 focus:ring-accent/20 disabled:opacity-55 disabled:hover:border-line'

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  mono?: boolean
  invalid?: boolean
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input({ mono, invalid, className, ...rest }, ref) {
  return (
    <input
      ref={ref}
      spellCheck={false}
      autoComplete="off"
      className={cx(CONTROL, mono && 'font-mono text-xs', invalid && 'border-danger focus:border-danger focus:ring-danger/20', className)}
      {...rest}
    />
  )
})

export interface TextareaProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  mono?: boolean
  invalid?: boolean
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { mono, invalid, className, rows = 4, ...rest },
  ref,
) {
  return (
    <textarea
      ref={ref}
      rows={rows}
      spellCheck={false}
      className={cx(
        CONTROL,
        'h-auto resize-y py-2 leading-relaxed',
        mono && 'font-mono text-xs',
        invalid && 'border-danger focus:border-danger focus:ring-danger/20',
        className,
      )}
      {...rest}
    />
  )
})

export type Option = string | { value: string; label: string; disabled?: boolean }

export interface SelectProps extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'onChange' | 'value'> {
  value: string
  onChange: (value: string) => void
  options: Option[]
  /** Shown as a first, empty option (e.g. "deployment default"). */
  placeholder?: string
  mono?: boolean
}

export function Select({ value, onChange, options, placeholder, mono, className, ...rest }: SelectProps) {
  return (
    <div className={cx('relative', className)}>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className={cx(CONTROL, 'cursor-pointer appearance-none pr-8', mono && 'font-mono text-xs', value === '' && placeholder && 'text-faint')}
        {...rest}
      >
        {placeholder !== undefined && <option value="">{placeholder}</option>}
        {options.map((o) => {
          const opt = typeof o === 'string' ? { value: o, label: o } : o
          return (
            <option key={opt.value} value={opt.value} disabled={opt.disabled}>
              {opt.label}
            </option>
          )
        })}
      </select>
      <ChevronDown className="pointer-events-none absolute right-2.5 top-1/2 size-3.5 -translate-y-1/2 text-faint" />
    </div>
  )
}

/** A text input that suggests from a list but accepts anything (ids, uuids). */
export function ComboInput({
  options,
  ...rest
}: InputProps & { options: string[] }) {
  const id = useId()
  return (
    <>
      <Input list={id} {...rest} />
      <datalist id={id}>
        {options.map((o) => (
          <option key={o} value={o} />
        ))}
      </datalist>
    </>
  )
}

export function Checkbox({
  checked,
  onChange,
  label,
  hint,
  disabled,
  mono,
  className,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label?: ReactNode
  hint?: ReactNode
  disabled?: boolean
  mono?: boolean
  className?: string
}) {
  return (
    <label className={cx('group flex min-w-0 select-none items-start gap-2', disabled ? 'cursor-not-allowed opacity-55' : 'cursor-pointer', className)}>
      <span className="relative mt-[2px] flex size-[15px] shrink-0 items-center justify-center">
        <input
          type="checkbox"
          checked={checked}
          disabled={disabled}
          onChange={(e) => onChange(e.target.checked)}
          className="peer absolute inset-0 cursor-[inherit] appearance-none rounded-[4px] border border-faint bg-bg transition-colors checked:border-accent checked:bg-accent group-hover:border-muted checked:group-hover:border-accent"
        />
        <Check className="pointer-events-none relative size-3 text-on-primary opacity-0 peer-checked:opacity-100" strokeWidth={3} />
      </span>
      {(label || hint) && (
        <span className="min-w-0">
          {label && <span className={cx('block truncate text-[13px] text-text', mono && 'font-mono text-xs')}>{label}</span>}
          {hint && <span className="block text-xs text-muted">{hint}</span>}
        </span>
      )}
    </label>
  )
}

export function Segmented<V extends string>({
  value,
  onChange,
  options,
  className,
}: {
  value: V
  onChange: (v: V) => void
  options: { value: V; label: ReactNode; disabled?: boolean; title?: string }[]
  className?: string
}) {
  return (
    <div className={cx('inline-flex rounded-ctl border border-line bg-bg p-0.5', className)} role="tablist">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          role="tab"
          aria-selected={o.value === value}
          disabled={o.disabled}
          title={o.title}
          onClick={() => onChange(o.value)}
          className={cx(
            'h-6 rounded-[6px] px-2.5 text-xs font-medium transition-colors duration-100 disabled:opacity-40',
            o.value === value ? 'bg-accent text-on-primary' : 'text-muted hover:bg-hover hover:text-text',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

/** A label over a control, with an optional hint or error beneath. */
export function Field({
  label,
  hint,
  error,
  children,
  className,
}: {
  label: ReactNode
  hint?: ReactNode
  error?: ReactNode
  children: ReactNode
  className?: string
}) {
  return (
    <div className={cx('flex min-w-0 flex-col gap-1', className)}>
      <span className="text-[11px] font-medium text-muted">{label}</span>
      {children}
      {error ? <span className="text-[11px] text-danger">{error}</span> : hint ? <span className="text-[11px] leading-snug text-faint">{hint}</span> : null}
    </div>
  )
}

export function SearchBox({
  value,
  onChange,
  placeholder = 'Filter…',
  className,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  placeholder?: string
  className?: string
  autoFocus?: boolean
}) {
  return (
    <div className={cx('relative', className)}>
      <Search className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-faint" />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        spellCheck={false}
        autoFocus={autoFocus}
        className={cx(CONTROL, 'pl-8 pr-7')}
      />
      {value !== '' && (
        <button
          type="button"
          aria-label="Clear"
          onClick={() => onChange('')}
          className="absolute right-1.5 top-1/2 flex size-5 -translate-y-1/2 items-center justify-center rounded text-faint hover:bg-hover hover:text-text"
        >
          <X className="size-3.5" />
        </button>
      )}
    </div>
  )
}

// ---- badges -----------------------------------------------------------------

/** A borderless tinted badge: the tone colors the text and the fill. */
export function Chip({ tone = 'neutral', children, className, title }: { tone?: Tone; children: ReactNode; className?: string; title?: string }) {
  return (
    <span
      title={title}
      className={cx(
        'inline-flex h-5 max-w-full shrink-0 items-center truncate rounded-md px-2 font-mono text-[10.5px] leading-none',
        TONE_BG[tone],
        TONE_TEXT[tone],
        className,
      )}
    >
      {children}
    </span>
  )
}

/** A chip led by a state dot: live, restart required, busy. */
export function StatusChip({ tone = 'neutral', children, pulse, className }: { tone?: Tone; children: ReactNode; pulse?: boolean; className?: string }) {
  return (
    <span className={cx('inline-flex h-5 shrink-0 items-center gap-1.5 rounded-full px-2 font-mono text-[10.5px] leading-none', TONE_BG[tone], TONE_TEXT[tone], className)}>
      <span className={cx('size-1.5 rounded-full', TONE_DOT[tone], pulse && 'animate-pulse')} />
      {children}
    </span>
  )
}

export function Dot({ tone = 'neutral', pulse, className }: { tone?: Tone; pulse?: boolean; className?: string }) {
  return <span className={cx('inline-block size-2 shrink-0 rounded-full', TONE_DOT[tone], pulse && 'animate-pulse', className)} />
}

export function Chips({ items, tone = 'neutral', max }: { items: string[]; tone?: Tone; max?: number }) {
  const shown = max && items.length > max ? items.slice(0, max) : items
  const extra = items.length - shown.length
  return (
    <span className="inline-flex min-w-0 flex-wrap items-center gap-1">
      {shown.map((it) => (
        <Chip key={it} tone={tone}>
          {it}
        </Chip>
      ))}
      {extra > 0 && <Chip title={items.slice(shown.length).join(', ')}>+{extra}</Chip>}
    </span>
  )
}

/** A keyboard shortcut drawn as a keycap. */
export function Kbd({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <kbd className={cx('inline-flex h-[18px] shrink-0 items-center rounded border border-line bg-surface2 px-1.5 font-mono text-[10px] leading-none text-muted', className)}>
      {children}
    </kbd>
  )
}

export function Avatar({ letter, size = 30, className }: { letter: string; size?: number; className?: string }) {
  return (
    <span
      className={cx('inline-flex shrink-0 items-center justify-center rounded-full border border-accent/60 bg-accent-dim font-semibold text-accent', className)}
      style={{ width: size, height: size, fontSize: size * 0.4 }}
    >
      {letter}
    </span>
  )
}

// ---- surfaces ---------------------------------------------------------------

/** A card. `flush` removes the padding for lists and tables that draw their own. */
export function Panel({ children, className, flush }: { children: ReactNode; className?: string; flush?: boolean }) {
  return <div className={cx('rounded-card border border-line bg-surface', !flush && 'p-3', className)}>{children}</div>
}

/** A titled card: heading, optional one-line explanation and a trailing action. */
export function Section({
  title,
  subtitle,
  action,
  children,
  className,
  flush,
}: {
  title: ReactNode
  subtitle?: ReactNode
  action?: ReactNode
  children?: ReactNode
  className?: string
  /** Lets the content reach the card's edges (tables). */
  flush?: boolean
}) {
  return (
    <section className={cx('flex min-w-0 flex-col rounded-card border border-line bg-surface', className)}>
      <header className="flex items-start gap-3 px-3 pb-2 pt-3">
        <div className="min-w-0 flex-1">
          <h3 className="text-[14px] font-semibold leading-tight text-text">{title}</h3>
          {subtitle && <p className="mt-0.5 text-xs leading-snug text-muted">{subtitle}</p>}
        </div>
        {action && <div className="flex shrink-0 items-center gap-1.5">{action}</div>}
      </header>
      {children !== undefined && <div className={cx('min-w-0', flush ? 'pb-1' : 'px-3 pb-3')}>{children}</div>}
    </section>
  )
}

export function Banner({ tone = 'danger', children, onClose, className }: { tone?: Tone; children: ReactNode; onClose?: () => void; className?: string }) {
  if (children === null || children === undefined || children === '') return null
  return (
    <div
      role={tone === 'danger' ? 'alert' : 'status'}
      className={cx('flex items-start gap-2 rounded-ctl border py-2 pl-3 pr-2 text-[12.5px] leading-snug', TONE_BG[tone], TONE_BORDER[tone], TONE_TEXT[tone], className)}
    >
      <span className={cx('mt-0.5 w-[3px] shrink-0 self-stretch rounded-full', TONE_DOT[tone])} />
      <div className="min-w-0 flex-1 whitespace-pre-wrap break-words">{children}</div>
      {onClose && (
        <button type="button" aria-label="Dismiss" onClick={onClose} className="rounded p-0.5 opacity-70 hover:opacity-100">
          <X className="size-3.5" />
        </button>
      )}
    </div>
  )
}

export function Spinner({ className }: { className?: string }) {
  return <Loader2 className={cx('size-4 animate-spin text-muted', className)} />
}

export function Loading({ text = 'Loading…', className }: { text?: string; className?: string }) {
  return (
    <div className={cx('flex items-center justify-center gap-2 py-10 text-muted', className)}>
      <Spinner />
      <span>{text}</span>
    </div>
  )
}

export function Logo({ size = 22, className }: { size?: number; className?: string }) {
  const id = useId()
  return (
    <svg width={size} height={size} viewBox="0 0 128 128" className={className} aria-hidden>
      <defs>
        <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="var(--af-logo-a)" />
          <stop offset="0.5" stopColor="var(--af-logo-b)" />
          <stop offset="1" stopColor="var(--af-logo-c)" />
        </linearGradient>
      </defs>
      <rect x="3" y="3" width="122" height="122" rx="30" fill="var(--af-surface)" stroke="var(--af-line)" strokeWidth="3" />
      <path d="M64 26 L100 102 H86 L64 60 L42 102 H28 Z" fill={`url(#${id})`} />
    </svg>
  )
}

export function EmptyState({
  title,
  guidance,
  actionLabel,
  onAction,
  icon,
  className,
}: {
  title: string
  guidance?: ReactNode
  actionLabel?: string
  onAction?: () => void
  icon?: ReactNode
  className?: string
}) {
  return (
    <div className={cx('flex flex-1 items-center justify-center py-10', className)}>
      <div className="flag-glow flex max-w-md flex-col items-center gap-2 rounded-card border border-line bg-surface px-8 py-7 text-center">
        {icon ?? <Logo size={48} />}
        <div className="mt-1 text-[14px] font-semibold">{title}</div>
        {guidance && <div className="whitespace-pre-wrap text-[12.5px] leading-relaxed text-muted">{guidance}</div>}
        {actionLabel && onAction && (
          <Button variant="primary" className="mt-2 min-w-44" onClick={onAction}>
            {actionLabel}
          </Button>
        )}
      </div>
    </div>
  )
}

/** A label over a monospace value. */
export function KV({ label, value, copy, className }: { label: string; value: ReactNode; copy?: string; className?: string }) {
  return (
    <div className={cx('min-w-0', className)}>
      <div className="text-[11px] text-muted">{label}</div>
      <div className="flex min-w-0 items-center gap-1">
        <span className="truncate font-mono text-xs text-text">{value}</span>
        {copy && <CopyButton text={copy} what={label} />}
      </div>
    </div>
  )
}

// ---- page scaffold ----------------------------------------------------------

/**
 * The standard page: a header with actions, an optional meta line and banner,
 * and a body. With `scroll` (the default) the body scrolls vertically; without
 * it the body is a fixed-height flex column for split views and editors.
 */
export function Page({
  title,
  subtitle,
  actions,
  meta,
  banner,
  children,
  scroll = true,
  className,
}: {
  title: ReactNode
  subtitle?: ReactNode
  actions?: ReactNode
  meta?: ReactNode
  banner?: ReactNode
  children: ReactNode
  scroll?: boolean
  className?: string
}) {
  return (
    <div className="flex h-full min-h-0 flex-col">
      <header className="shrink-0 border-b border-line px-5 pb-3 pt-3">
        <div className="flex min-h-[30px] items-center gap-3">
          <h1 className="min-w-0 truncate text-[19px] font-semibold leading-tight tracking-[-0.01em]">{title}</h1>
          <div className="flex-1" />
          {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
        </div>
        {subtitle && <p className="mt-0.5 text-[12.5px] leading-snug text-muted">{subtitle}</p>}
        {meta && <div className="mt-1.5 flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 font-mono text-[11px] text-faint">{meta}</div>}
        {banner && <div className="mt-2 flex flex-col gap-2 empty:hidden">{banner}</div>}
      </header>
      <div className={cx('min-h-0 flex-1', scroll ? 'overflow-y-auto' : 'flex flex-col overflow-hidden', 'px-5 py-3', className)}>
        {children}
      </div>
    </div>
  )
}

/**
 * Renders the three states every screen has besides its content. A subsystem
 * that is switched off on the runtime (503) reads as guidance, not an error.
 */
export function AsyncView({
  loading,
  error,
  hasData,
  retry,
  loadingText,
  children,
}: {
  loading: boolean
  error: unknown
  hasData: boolean
  retry?: () => void
  loadingText?: string
  children: ReactNode
}) {
  if (loading && !hasData) return <Loading text={loadingText} />
  if (error && !hasData) {
    const off = featureOff(error)
    if (off) return <EmptyState title="Not enabled on this runtime" guidance={off} />
    return (
      <div className="flex flex-col items-center gap-3 py-6">
        <Banner tone="danger" className="w-full">
          {errorMessage(error)}
        </Banner>
        {retry && (
          <Button icon={RotateCw} onClick={retry}>
            Retry
          </Button>
        )}
      </div>
    )
  }
  return <>{children}</>
}

// ---- data presentation ------------------------------------------------------

export interface Column<R> {
  key: string
  title: ReactNode
  /** CSS width (e.g. 120 or '20%'); omit for a flexible column. */
  width?: number | string
  align?: 'left' | 'right'
  mono?: boolean
  render: (row: R, index: number) => ReactNode
}

/** A dense table with a quiet header and hairline row separators. */
export function Table<R>({
  columns,
  rows,
  rowKey,
  onRowClick,
  selected,
  empty,
  className,
}: {
  columns: Column<R>[]
  rows: R[]
  rowKey: (row: R, index: number) => string
  onRowClick?: (row: R) => void
  selected?: (row: R) => boolean
  empty?: ReactNode
  className?: string
}) {
  if (rows.length === 0 && empty !== undefined) {
    return <div className="px-3 py-6 text-center text-[12.5px] text-muted">{empty}</div>
  }
  return (
    <div className={cx('min-w-0 overflow-x-auto', className)}>
      <table className="w-full table-fixed border-collapse text-[12.5px]">
        <colgroup>
          {columns.map((c) => (
            <col key={c.key} style={c.width !== undefined ? { width: c.width } : undefined} />
          ))}
        </colgroup>
        <thead>
          <tr>
            {columns.map((c) => (
              <th
                key={c.key}
                className={cx(
                  'border-b border-line px-3 py-1.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-faint',
                  c.align === 'right' ? 'text-right' : 'text-left',
                )}
              >
                {c.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row, i) => (
            <tr
              key={rowKey(row, i)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={cx(
                'border-b border-line/70 last:border-b-0',
                onRowClick && 'cursor-pointer hover:bg-hover',
                selected?.(row) && 'bg-accent-dim/60',
              )}
            >
              {columns.map((c) => (
                <td
                  key={c.key}
                  className={cx('truncate px-3 py-1.5 align-middle', c.align === 'right' && 'text-right tabular-nums', c.mono && 'font-mono text-xs')}
                >
                  {c.render(row, i)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

/** A counter series as a polyline scaled to its own range. */
export function Sparkline({ values, height = 28, className }: { values: number[]; height?: number; className?: string }) {
  if (values.length < 2) return <div style={{ height }} className={className} />
  const lo = Math.min(...values)
  const hi = Math.max(...values)
  // Scale to the series' own range, but never magnify a change smaller than a
  // tenth of the value: a counter ticking 4→6 should read as nearly flat.
  const span = Math.max(hi - lo, hi / 10, 1)
  const w = 100
  const h = height - 2
  const pts = values.map((v, i) => `${((i / (values.length - 1)) * w).toFixed(2)},${Math.max(1, h - ((v - lo) / span) * h + 1).toFixed(2)}`)
  const id = useId()
  return (
    <svg viewBox={`0 0 ${w} ${height}`} preserveAspectRatio="none" className={cx('w-full', className)} style={{ height }} aria-hidden>
      <defs>
        <linearGradient id={id} x1="0" x2="1" y1="0" y2="0">
          <stop offset="0" stopColor="var(--af-accent)" />
          <stop offset="1" stopColor="var(--af-accent2)" />
        </linearGradient>
      </defs>
      <polyline points={pts.join(' ')} fill="none" stroke={`url(#${id})`} strokeWidth="1.5" vectorEffect="non-scaling-stroke" strokeLinejoin="round" strokeLinecap="round" />
    </svg>
  )
}

/** A compact KPI: label, big value, optional sub-line and sparkline. */
export function StatTile({
  label,
  value,
  sub,
  tone = 'neutral',
  spark,
  className,
}: {
  label: string
  value: ReactNode
  sub?: ReactNode
  tone?: Tone
  spark?: number[]
  className?: string
}) {
  return (
    <div className={cx('flex min-w-0 flex-col rounded-card border border-line bg-surface px-3 py-2.5', className)}>
      <div className="truncate text-[10px] font-semibold uppercase tracking-[0.06em] text-faint">{label}</div>
      <div className={cx('mt-1 truncate text-[20px] font-semibold leading-tight tabular-nums', tone === 'neutral' ? 'text-text' : TONE_TEXT[tone])}>{value}</div>
      {sub && <div className="mt-0.5 truncate text-[11px] text-muted">{sub}</div>}
      {spark && <Sparkline values={spark} className="mt-1.5" />}
    </div>
  )
}

/** Used against a limit; limit <= 0 renders an "unlimited" chip. */
export function Meter({ used, limit, className }: { used: number; limit: number; className?: string }) {
  if (limit <= 0) {
    return (
      <span className={cx('inline-flex items-center gap-1.5', className)}>
        <span className="font-mono text-xs">{humanCount(used)}</span>
        <Chip>unlimited</Chip>
      </span>
    )
  }
  const frac = Math.min(used / limit, 1)
  const tone = frac >= 1 ? 'bg-danger' : frac >= 0.8 ? 'bg-warning' : 'bg-accent'
  return (
    <div className={cx('min-w-0', className)}>
      <div className="h-1.5 overflow-hidden rounded-full bg-surface2">
        <div className={cx('h-full rounded-full transition-[width]', tone)} style={{ width: `${Math.max(frac * 100, used > 0 ? 2 : 0)}%` }} />
      </div>
      <div className="mt-1 font-mono text-[11px] text-muted">
        {humanCount(used)} / {humanCount(limit)}
      </div>
    </div>
  )
}

export interface TabDef<V extends string> {
  id: V
  label: ReactNode
  icon?: LucideIcon
  badge?: ReactNode
}

/** An underlined tab strip; the active tab carries the flag gradient. */
export function Tabs<V extends string>({
  tabs,
  value,
  onChange,
  trailing,
  className,
}: {
  tabs: TabDef<V>[]
  value: V
  onChange: (v: V) => void
  trailing?: ReactNode
  className?: string
}) {
  return (
    <div className={cx('flex shrink-0 items-center gap-1 border-b border-line', className)} role="tablist">
      {tabs.map((t) => {
        const active = t.id === value
        return (
          <button
            key={t.id}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => onChange(t.id)}
            className={cx(
              'relative -mb-px flex h-8 items-center gap-1.5 px-2.5 text-[13px] transition-colors',
              active ? 'font-semibold text-text' : 'text-muted hover:text-text',
            )}
          >
            {t.icon && <t.icon className="size-3.5" strokeWidth={2} />}
            {t.label}
            {t.badge}
            {active && <span className="absolute inset-x-1.5 bottom-0 h-0.5 rounded-full bg-gradient-to-r from-accent to-accent2" />}
          </button>
        )
      })}
      {trailing && <div className="ml-auto flex items-center gap-2 pb-1">{trailing}</div>}
    </div>
  )
}

// ---- modal ------------------------------------------------------------------

/**
 * A centered modal card. Esc and the backdrop close it; focus moves into the
 * card on open and returns to the opener on close.
 */
export function Modal({
  open,
  onClose,
  title,
  subtitle,
  children,
  footer,
  width = 560,
  dismissable = true,
}: {
  open: boolean
  onClose: () => void
  title: ReactNode
  subtitle?: ReactNode
  children: ReactNode
  footer?: ReactNode
  width?: number
  /** False keeps the modal open on Esc/backdrop (e.g. while saving). */
  dismissable?: boolean
}) {
  const card = useRef<HTMLDivElement>(null)
  const backdrop = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  onCloseRef.current = onClose
  const dismissableRef = useRef(dismissable)
  dismissableRef.current = dismissable
  useEffect(() => {
    if (!open) return
    const active = document.activeElement as HTMLElement | null
    // React's autoFocus has already run by now: when it put focus inside the
    // card, that choice stands.
    const inside = !!active && !!card.current?.contains(active)
    const opener = inside ? null : active
    if (!inside) {
      const first = card.current?.querySelector<HTMLElement>('input:not([type=hidden]), textarea, select')
      ;(first ?? card.current)?.focus()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      // With a dialog open over a modal, Esc belongs to the topmost one only.
      const all = document.querySelectorAll('[data-modal]')
      if (all[all.length - 1] !== backdrop.current) return
      e.stopImmediatePropagation()
      if (dismissableRef.current) onCloseRef.current()
    }
    document.addEventListener('keydown', onKey, true)
    return () => {
      document.removeEventListener('keydown', onKey, true)
      opener?.focus?.()
    }
    // dismissable is read through a ref: toggling it while saving must not
    // re-run this effect and move the focus.
  }, [open])
  if (!open) return null
  return createPortal(
    <div
      ref={backdrop}
      className="fixed inset-0 z-50 flex animate-fade-in items-start justify-center overflow-y-auto bg-black/55 px-4 py-[8vh] backdrop-blur-[2px]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && dismissable) onClose()
      }}
      data-modal
    >
      <div
        ref={card}
        role="dialog"
        aria-modal="true"
        tabIndex={-1}
        style={{ width, maxWidth: '100%' }}
        className="flex max-h-[84vh] animate-pop-in flex-col rounded-xl border border-line bg-surface shadow-float outline-none"
      >
        <header className="flex shrink-0 items-start gap-3 px-5 pb-3 pt-4">
          <div className="min-w-0 flex-1">
            <h2 className="text-[16px] font-semibold leading-tight">{title}</h2>
            {subtitle && <p className="mt-1 text-[12.5px] leading-snug text-muted">{subtitle}</p>}
          </div>
          <IconButton icon={X} label="Close" onClick={onClose} disabled={!dismissable} />
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto border-t border-line px-5 py-4">{children}</div>
        {footer && <footer className="flex shrink-0 items-center justify-end gap-2 border-t border-line px-5 py-3">{footer}</footer>}
      </div>
    </div>,
    document.body,
  )
}

/** True when a modal (or the palette) is open, so global shortcuts stand down. */
export function modalOpen(): boolean {
  return document.querySelector('[data-modal]') !== null
}
