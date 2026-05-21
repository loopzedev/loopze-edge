// Thin fetch helpers for the dashboard SPA.
//
// The dashboard uses session cookies for auth (same as the editor) — every
// request must include credentials. Origin is always same-origin (SPA is
// served by the LOOPZE backend), so no CORS dance is needed.

import type { Snapshot } from './types'

export interface Theme {
  name: string
  theme: string
  accentColor: string
  density: string
  showNav: boolean
}

export async function fetchLayout(): Promise<Snapshot> {
  const res = await fetch('/api/dashboard/layout', { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`layout: HTTP ${res.status}`)
  }
  return res.json()
}

export async function fetchTheme(): Promise<Theme> {
  const res = await fetch('/api/dashboard/theme', { credentials: 'include' })
  if (!res.ok) {
    throw new Error(`theme: HTTP ${res.status}`)
  }
  return res.json()
}
