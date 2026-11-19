// The message syntax both Studio apps share (the desktop twin is
// desktop/internal/i18n/format.go; keep the two in step).
//
//   {name}                                    a parameter
//   {n, plural, =0 {…} one {…} other {…}}     a count; # prints it
//   {kind, select, a {…} b {…} other {…}}     a choice
//
// A brace that does not open one of those forms is literal text, so JSON and
// YAML samples can appear in a message as they are.

export type Params = Record<string, string | number | boolean | null | undefined>

type PluralCategory = (n: number) => string

const IDENT = /^[A-Za-z_][A-Za-z0-9_]*/

/** The end of the brace group opening at `open`, or -1 when it never closes. */
function closing(s: string, open: number): number {
  let depth = 0
  for (let i = open; i < s.length; i++) {
    if (s[i] === '{') depth++
    else if (s[i] === '}') {
      depth--
      if (depth === 0) return i
    }
  }
  return -1
}

/** Parses "key {text} key {text}" into its branches. */
function branches(s: string): Map<string, string> | null {
  const out = new Map<string, string>()
  let i = 0
  for (;;) {
    while (i < s.length && /\s/.test(s[i])) i++
    if (i >= s.length) return out.size ? out : null
    let j = i
    while (j < s.length && s[j] !== '{' && !/\s/.test(s[j])) j++
    const key = s.slice(i, j)
    while (j < s.length && /\s/.test(s[j])) j++
    if (!key || s[j] !== '{') return null
    const end = closing(s, j)
    if (end < 0) return null
    out.set(key, s.slice(j + 1, end))
    i = end + 1
  }
}

export function formatMessage(msg: string, params: Params | undefined, category: PluralCategory, hash?: string): string {
  let out = ''
  let i = 0
  while (i < msg.length) {
    const ch = msg[i]
    if (ch === '#' && hash !== undefined) {
      out += hash
      i++
      continue
    }
    if (ch !== '{') {
      out += ch
      i++
      continue
    }
    const end = closing(msg, i)
    const body = end < 0 ? '' : msg.slice(i + 1, end)
    const name = IDENT.exec(body)?.[0]
    if (!name) {
      out += ch
      i++
      continue
    }
    const rest = body.slice(name.length)
    const value = params?.[name]
    if (rest === '') {
      out += value === undefined || value === null ? '{' + name + '}' : String(value)
      i = end + 1
      continue
    }
    const m = /^\s*,\s*(plural|select)\s*,/.exec(rest)
    const opts = m ? branches(rest.slice(m[0].length)) : null
    if (!m || !opts) {
      out += ch
      i++
      continue
    }
    let pick: string | undefined
    if (m[1] === 'plural') {
      const n = Number(value ?? 0)
      pick = opts.get('=' + n) ?? opts.get(category(n)) ?? opts.get('other')
      out += formatMessage(pick ?? '', params, category, String(value ?? 0))
    } else {
      pick = opts.get(String(value)) ?? opts.get('other')
      out += formatMessage(pick ?? '', params, category, hash)
    }
    i = end + 1
  }
  return out
}

/** The parameter names a message reads, for the catalogue checks. */
export function messageParams(msg: string): string[] {
  const seen = new Set<string>()
  const walk = (s: string) => {
    let i = 0
    while (i < s.length) {
      if (s[i] !== '{') {
        i++
        continue
      }
      const end = closing(s, i)
      const body = end < 0 ? '' : s.slice(i + 1, end)
      const name = IDENT.exec(body)?.[0]
      if (!name) {
        i++
        continue
      }
      const rest = body.slice(name.length)
      if (rest === '') {
        seen.add(name)
        i = end + 1
        continue
      }
      const m = /^\s*,\s*(plural|select)\s*,/.exec(rest)
      const opts = m ? branches(rest.slice(m[0].length)) : null
      if (!m || !opts) {
        i++
        continue
      }
      seen.add(name)
      for (const text of opts.values()) walk(text)
      i = end + 1
    }
  }
  walk(msg)
  return [...seen].sort()
}
