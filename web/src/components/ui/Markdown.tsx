// Renders bundled Markdown (the documentation and the builder's inline guide).
// The content is compiled into the app, never fetched or user-supplied, so the
// HTML marked produces is trusted. Links are intercepted: a link to another
// guide navigates inside the viewer, an anchor scrolls, and anything external
// opens in a new tab.
import { useEffect, useMemo, useRef } from 'react'
import { Marked } from 'marked'
import { assetFor, headingID, slugFor } from '@/docs/library'
import { copyText } from '@/lib/format'
import { toast } from '@/state/ui'
import { cx } from './index'

function escapeHTML(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
}

const words = (list: string) => new Set(list.split(' '))

const KEYWORDS: Record<string, Set<string>> = {
  lua: words('and break continue do else elseif end export false for function if in local nil not or repeat return then true type until while'),
  yaml: words('true false null yes no on off'),
  bash: words('if then else fi for do done export echo curl cd make go git agentflow'),
  json: words('true false null'),
}

/**
 * A small highlighter for the fenced blocks the guides use (lua, yaml, bash,
 * json). It marks comments, strings, numbers and keywords; anything else is
 * left as plain text.
 */
export function highlightCode(code: string, lang: string): string {
  const l = lang.toLowerCase().replace(/^luau$/, 'lua').replace(/^(sh|shell|console)$/, 'bash').replace(/^yml$/, 'yaml')
  const kw = KEYWORDS[l]
  if (!kw) return escapeHTML(code)
  const comment = l === 'lua' ? /--\[\[[\s\S]*?\]\]|--[^\n]*/y : l === 'json' ? null : /#[^\n]*/y
  const string = /"(?:[^"\\\n]|\\.)*"|'(?:[^'\\\n]|\\.)*'/y
  const number = /\b\d+(?:\.\d+)?\b/y
  const word = /[A-Za-z_][A-Za-z0-9_]*/y
  let out = ''
  let i = 0
  const span = (color: string, text: string, italic = false) =>
    `<span style="color:var(${color})${italic ? ';font-style:italic' : ''}">${escapeHTML(text)}</span>`
  const at = (re: RegExp) => {
    re.lastIndex = i
    return re.exec(code)
  }
  while (i < code.length) {
    let m: RegExpExecArray | null
    // A '#' only starts a comment at line start or after whitespace.
    const commentOK = comment && (l === 'lua' || i === 0 || /\s/.test(code[i - 1]))
    if (commentOK && (m = at(comment!))) {
      out += span('--af-faint', m[0], true)
    } else if ((m = at(string))) {
      out += span('--af-success', m[0])
    } else if ((m = at(number))) {
      out += span('--af-warning', m[0])
    } else if ((m = at(word))) {
      if (kw.has(m[0])) out += span('--af-accent2', m[0])
      else if (l === 'yaml' && /^\s*:/.test(code.slice(i + m[0].length))) out += span('--af-accent', m[0])
      else out += escapeHTML(m[0])
    } else {
      out += escapeHTML(code[i])
      i++
      continue
    }
    i += m![0].length
  }
  return out
}

/** Whether a link leads somewhere this viewer can take the reader. */
function resolvable(href: string): boolean {
  return href.startsWith('#') || slugFor(href) !== undefined || assetFor(href) !== undefined || /^(https?:)?\/\//.test(href) || href.startsWith('mailto:')
}

function renderer(): Marked {
  const md = new Marked({ gfm: true, breaks: false })
  md.use({
    renderer: {
      // A relative link to a file that is not bundled reads as plain text, as
      // in the desktop viewer, instead of a link that does nothing.
      link({ href, tokens }) {
        return resolvable(href) ? false : this.parser.parseInline(tokens)
      },
      heading({ tokens, depth }) {
        const html = this.parser.parseInline(tokens)
        const text = html.replace(/<[^>]+>/g, '')
        const id = headingID(text.replace(/&amp;/g, '&').replace(/&lt;/g, '<').replace(/&gt;/g, '>').replace(/&quot;/g, '"').replace(/&#39;/g, "'"))
        return `<h${depth} id="${escapeHTML(id)}">${html}</h${depth}>\n`
      },
      code({ text, lang }) {
        const language = (lang ?? '').trim().split(/\s+/)[0]
        return `<pre data-lang="${escapeHTML(language)}"><code>${highlightCode(text, language)}</code></pre>\n`
      },
    },
  })
  return md
}

const marked = renderer()

export function Markdown({
  source,
  compact,
  onNavigate,
  onAsset,
  className,
}: {
  source: string
  /** Tighter type scale for side panels. */
  compact?: boolean
  /** Called for links to another bundled guide (slug, optional #anchor). */
  onNavigate?: (slug: string, anchor?: string) => void
  /** Called for links to a bundled contract file (openapi/users.yaml, ...). */
  onAsset?: (path: string) => void
  className?: string
}) {
  const html = useMemo(() => marked.parse(source, { async: false }) as string, [source])
  const root = useRef<HTMLDivElement>(null)

  // Code blocks get a copy button without re-rendering the HTML.
  useEffect(() => {
    const el = root.current
    if (!el) return
    for (const pre of Array.from(el.querySelectorAll('pre'))) {
      if (pre.querySelector('[data-copy]')) continue
      pre.style.position = 'relative'
      const btn = document.createElement('button')
      btn.type = 'button'
      btn.dataset.copy = '1'
      btn.textContent = 'copy'
      btn.className =
        'absolute right-1.5 top-1.5 rounded border border-line bg-surface2 px-1.5 py-0.5 font-mono text-xs text-muted opacity-0 transition-opacity hover:text-text'
      btn.onclick = async () => {
        const code = pre.querySelector('code')?.textContent ?? ''
        if (await copyText(code)) toast('code copied', 'success')
      }
      pre.addEventListener('mouseenter', () => (btn.style.opacity = '1'))
      pre.addEventListener('mouseleave', () => (btn.style.opacity = '0'))
      pre.appendChild(btn)
    }
  }, [html])

  return (
    <div
      ref={root}
      className={cx('prose-af', compact && 'compact', className)}
      dangerouslySetInnerHTML={{ __html: html }}
      onClick={(e) => {
        const a = (e.target as HTMLElement).closest('a')
        if (!a) return
        const href = a.getAttribute('href') ?? ''
        if (href.startsWith('#')) {
          e.preventDefault()
          root.current?.querySelector(`[id="${CSS.escape(decodeURIComponent(href.slice(1)))}"]`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
          return
        }
        const slug = slugFor(href)
        if (slug) {
          e.preventDefault()
          const hash = href.indexOf('#')
          onNavigate?.(slug, hash >= 0 ? decodeURIComponent(href.slice(hash + 1)) : undefined)
          return
        }
        const asset = assetFor(href)
        if (asset) {
          e.preventDefault()
          onAsset?.(asset.path)
          return
        }
        if (/^(https?:)?\/\//.test(href) || href.startsWith('mailto:')) {
          e.preventDefault()
          window.open(href, '_blank', 'noopener,noreferrer')
          return
        }
        // A relative link to something that is not bundled goes nowhere useful.
        e.preventDefault()
      }}
    />
  )
}
