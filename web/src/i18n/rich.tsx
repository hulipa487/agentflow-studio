// tr(): a message with markup. The catalogue marks spans with tags,
// "Type <code>{name}</code> to confirm", and the caller says what each tag
// renders as, so word order stays the translator's and the markup the
// component's.
import { Fragment, type ReactNode } from 'react'
import { t, type Params } from './index'

export type Tags = Record<string, (children: ReactNode) => ReactNode>

// Stands in for "<" inside parameter values while the tags are being read, so
// a value such as an agent called "<b>" is never taken for markup.
const LT = ''

const TAG = /<([A-Za-z][A-Za-z0-9]*)>([\s\S]*?)<\/\1>/g

function render(text: string, tags: Tags, keyPrefix: string): ReactNode[] {
  const out: ReactNode[] = []
  let last = 0
  let n = 0
  for (const m of text.matchAll(TAG)) {
    const at = m.index ?? 0
    if (at > last) out.push(text.slice(last, at).replaceAll(LT, '<'))
    const wrap = tags[m[1]]
    const key = `${keyPrefix}${n++}`
    const inner = render(m[2], tags, key + '.')
    out.push(<Fragment key={key}>{wrap ? wrap(inner) : inner}</Fragment>)
    last = at + m[0].length
  }
  if (last < text.length) out.push(text.slice(last).replaceAll(LT, '<'))
  return out
}

export function tr(key: string, params: Params | undefined, tags: Tags): ReactNode {
  const safe: Params = {}
  for (const [name, value] of Object.entries(params ?? {})) {
    safe[name] = typeof value === 'string' ? value.replaceAll('<', LT) : value
  }
  return <>{render(t(key, safe), tags, '')}</>
}
