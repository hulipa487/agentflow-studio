import type { Local, Model } from './analysis'
import {
  children,
  isKnownGlobal,
  leafName,
  lookup,
  stdlibCompletion,
  stdlibModules,
  type ApiEntry,
} from './api'
import { before, isIdentRune, type Position } from './token'

export type CompletionKind = 'function' | 'field' | 'variable' | 'module' | 'snippet'

/** One suggestion for the editor. */
export interface CompletionItem {
  label: string
  kind: CompletionKind
  detail: string
  documentation: string
  /** Text to insert, in LSP snippet syntax; see stripSnippet. */
  insert: string
  /** The prelude entry behind the item, when it is one. */
  entry?: ApiEntry
}

/** The documentation for the symbol at a position. */
export interface Hover {
  line: number
  col: number
  endLine: number
  endCol: number
  markdown: string
  /** The prelude entry the symbol resolves to, when the path is documented. */
  entry?: ApiEntry
}

/** What is being completed at a position. */
export interface CompletionScope {
  /** Code-point offsets of the typed prefix. */
  start: number
  end: number
  /** The identifier already typed before the cursor. */
  prefix: string
  /** The dotted path before the prefix ("session.state"), or "" at the root. */
  path: string
}

/** Reads the typed prefix and the dotted path before it from the source text. */
export function completionScope(m: Model, line: number, col: number): CompletionScope {
  const src = m.text.chars
  const off = m.text.offset(line, col)
  let start = off
  while (start > 0 && isIdentRune(src[start - 1])) start--

  let path = ''
  if (start > 0 && (src[start - 1] === '.' || src[start - 1] === ':')) {
    const dot = start - 1
    let p = dot
    while (p > 0 && isIdentRune(src[p - 1])) p--
    path = m.text.slice(p, dot)
    // Walk further back through a dotted chain (session.state.).
    while (p > 0 && src[p - 1] === '.') {
      let q = p - 1
      while (q > 0 && isIdentRune(src[q - 1])) q--
      if (q === p - 1) break
      path = m.text.slice(q, p - 1) + '.' + path
      p = q
    }
  }
  return { start, end: off, prefix: m.text.slice(start, off), path }
}

/**
 * Returns suggestions at a line/column. The prefix is taken from the text
 * before the cursor; after a dot or colon it lists the fields of the prelude
 * namespace named there.
 */
export function complete(m: Model | null | undefined, line: number, col: number): CompletionItem[] {
  if (!m) return []
  const scope = completionScope(m, line, col)
  const prefix = scope.prefix.toLowerCase()
  const path = scope.path

  // A local variable shadows a prelude namespace of the same name.
  if (path !== '' && localNamed(m, path.split('.')[0], line, col)) return []

  const items: CompletionItem[] = []
  const seen = new Set<string>()
  const add = (it: CompletionItem) => {
    if (it.label === '' || seen.has(it.label)) return
    if (!it.label.toLowerCase().startsWith(prefix)) return
    seen.add(it.label)
    items.push(it)
  }
  const byLabel = (a: CompletionItem, b: CompletionItem) => (a.label < b.label ? -1 : a.label > b.label ? 1 : 0)

  if (path !== '') {
    for (const e of children(path)) add(completionFromEntry(e))
    return items.sort(byLabel)
  }

  // Locals in scope win over globals.
  const pos: Position = { line, col }
  for (const l of m.locals) {
    if (before(pos, l.decl) || !inScope(l, line, col)) continue
    add({ label: l.name, kind: 'variable', detail: l.kind, documentation: '', insert: l.name })
  }
  for (const e of children('')) add(completionFromEntry(e))
  for (const name of stdlibCompletion()) {
    add({
      label: name,
      kind: stdlibModules.has(name) ? 'module' : 'function',
      detail: 'Luau standard library',
      documentation: '',
      insert: name,
    })
  }
  for (const name of m.globals) {
    add({ label: name, kind: 'function', detail: 'declared in this chunk', documentation: '', insert: name })
  }
  return items.sort(byLabel)
}

export function completionFromEntry(e: ApiEntry): CompletionItem {
  const label = leafName(e.path)
  let kind: CompletionKind = 'field'
  if (e.kind === 'function') kind = 'function'
  else if (e.kind === 'table') kind = 'module'
  else if (e.kind === 'variable') kind = 'variable'
  let doc = e.summary
  if (e.capability) doc += '\n\ncapability: ' + e.capability
  if (e.maintenance) doc += '\n\nmaintenance provenance only'
  return {
    label,
    kind,
    detail: e.signature,
    documentation: doc,
    insert: e.insert || label,
    entry: e,
  }
}

function localNamed(m: Model, name: string, line: number, col: number): boolean {
  return m.locals.some((l) => l.name === name && inScope(l, line, col))
}

function inScope(l: Local, line: number, col: number): boolean {
  const p: Position = { line, col }
  if (before(p, l.start)) return false
  if ((l.end.line !== 0 || l.end.col !== 0) && before(l.end, p)) return false
  return true
}

const localKindLabel: Record<Local['kind'], string> = {
  local: 'local variable',
  param: 'parameter',
  loop: 'loop variable',
  self: 'implicit method receiver',
  function: 'local function',
}

/** Describes the symbol at a line/column. */
export function hoverAtPosition(m: Model | null | undefined, line: number, col: number): Hover | null {
  if (!m) return null
  const tokens = m.tokens
  let idx = -1
  let lastOnLine = -1
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i]
    if (t.kind === 'eof') break
    if (t.line === line && t.col <= col && col < t.endCol) {
      idx = i
      break
    }
    if (t.line < line && t.endLine === line && col < t.endCol) {
      idx = i
      break
    }
    if (t.endLine === line && t.endCol <= col) {
      lastOnLine = i
      continue
    }
    if (t.line > line) break
  }
  if (idx < 0) idx = lastOnLine
  if (idx < 0 || tokens[idx].kind !== 'name') return null

  let start = idx
  while (
    start >= 2 &&
    (tokens[start - 1].lit === '.' || tokens[start - 1].lit === ':') &&
    (tokens[start - 2].kind === 'name' || tokens[start - 2].kind === 'keyword')
  ) {
    start -= 2
  }
  let end = idx
  while (
    end + 2 < tokens.length &&
    (tokens[end + 1].lit === '.' || tokens[end + 1].lit === ':') &&
    tokens[end + 2].kind === 'name'
  ) {
    end += 2
  }
  const parts: string[] = []
  for (let i = start; i <= end; i += 2) parts.push(tokens[i].lit)
  const path = parts.join('.')
  const root = parts[0]

  const range = {
    line: tokens[start].line,
    col: tokens[start].col,
    endLine: tokens[end].endLine,
    endCol: tokens[end].endCol,
  }

  const local = m.locals.find((l) => l.name === root && inScope(l, line, col))
  if (local) {
    return {
      ...range,
      markdown: `**${local.name}**\n\n${localKindLabel[local.kind] ?? 'local variable'}, declared at line ${local.decl.line + 1}.`,
    }
  }
  const entry = lookup(path)
  if (entry) return { ...range, markdown: entryMarkdown(entry), entry }
  const rootEntry = lookup(root)
  if (rootEntry) return { ...range, markdown: `**${parts.join('.')}**\n\n${rootEntry.summary}` }
  if (m.globals.has(root)) return { ...range, markdown: `**${root}**\n\nfunction declared in this chunk.` }
  if (isKnownGlobal(root)) return { ...range, markdown: `**${root}**\n\nLuau standard library.` }
  return null
}

/** The extra hover line for llm.stream, the one call with a usage trap. */
export const streamNote = 'Call `next_delta()` directly; a generic `for` fails across the bridge.'

export function entryMarkdown(e: ApiEntry): string {
  let b = `**${e.signature}**\n\n${e.summary}`
  if (e.capability) b += `\n\n*capability:* \`${e.capability}\``
  if (e.maintenance) b += '\n\n*maintenance provenance only (system or scheduler turns)*'
  if (e.path === 'llm.stream') b += '\n\n' + streamNote
  return b
}

/** Renders hover Markdown as a single plain line. */
export function plainDoc(md: string): string {
  return md
    .replaceAll('\n\n', ' · ')
    .replaceAll('\n', ' ')
    .replaceAll('**', '')
    .replaceAll('`', '')
    .replaceAll('*capability:*', 'capability:')
    .replaceAll('*', '')
    .trim()
}

/**
 * Reduces an LSP snippet to its default text: ${1:name} becomes name, bare
 * tab stops ($0, ${2}) disappear. The editor inserts this text as typed.
 */
export function stripSnippet(s: string): string {
  const rs = Array.from(s)
  let b = ''
  for (let i = 0; i < rs.length; i++) {
    if (rs[i] !== '$') {
      b += rs[i]
      continue
    }
    if (rs[i + 1] === '{') {
      let j = i + 2
      while (j < rs.length && rs[j] !== ':' && rs[j] !== '}') j++
      if (j < rs.length && rs[j] === ':') {
        let k = j + 1
        let depth = 0
        while (k < rs.length) {
          if (rs[k] === '{') {
            depth++
          } else if (rs[k] === '}') {
            if (depth === 0) break
            depth--
          }
          k++
        }
        if (k < rs.length) {
          b += rs.slice(j + 1, k).join('')
          i = k
          continue
        }
      }
      while (j < rs.length && rs[j] !== '}') j++
      i = j
      continue
    }
    if (i + 1 < rs.length && rs[i + 1] >= '0' && rs[i + 1] <= '9') {
      let j = i + 1
      while (j < rs.length && rs[j] >= '0' && rs[j] <= '9') j++
      i = j - 1
      continue
    }
    b += rs[i]
  }
  return b
}
