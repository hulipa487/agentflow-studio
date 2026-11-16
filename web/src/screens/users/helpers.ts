// Small pieces the Users and Tenancy screens share, ported from the desktop
// Studio's internal/ui/users.go.
import { useEffect, useRef, type RefObject } from 'react'
import type { UserRow } from '@/api/types'
import { humanCount, shortID } from '@/lib/format'
import type { Tone } from '@/state/ui'

export interface Notice {
  text: string
  tone: Tone
}

/** How a profile is named everywhere: display name, else email, else the id. */
export function userTitle(u: Pick<UserRow, 'display_name' | 'email' | 'user_id'>): string {
  return u.display_name || u.email || u.user_id
}

/** Ordinal comparison, so lists sort exactly as the desktop app sorts them. */
export function byText(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

/** The line under a profile's name in the list: email · id · quota · handles. */
export function userSummary(u: UserRow): string {
  const title = userTitle(u)
  let sub = shortID(u.user_id)
  if (u.email && u.email !== title) sub = u.email + ' · ' + sub
  const used = u.quota ? u.quota.used : 0
  const limit = u.quota ? u.quota.limit : u.tokens_per_day
  const quota = limit > 0 ? `${humanCount(used)} / ${humanCount(limit)}` : `${humanCount(used)} today`
  const handles = u.identities === 1 ? '1 handle' : `${u.identities ?? 0} handles`
  return `${sub} · ${quota} · ${handles}`
}

/** Quotes a value the way the desktop's messages do ("%q"). */
export function quoted(s: string): string {
  return JSON.stringify(s)
}

/** A whole number as Go's strconv parses it; null when it is not one. */
export function parseWhole(s: string): number | null {
  if (!/^[+-]?\d+$/.test(s)) return null
  const n = Number(s)
  return Number.isSafeInteger(n) ? n : null
}

/**
 * True while the component is mounted. Checked after an await, before touching
 * state: a request can outlive the view that started it.
 */
export function useAlive(): RefObject<boolean> {
  const alive = useRef(true)
  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])
  return alive
}
