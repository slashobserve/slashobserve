import type { InboxItem } from "./inbox-item"

const maxLiveToasts = 3
export const liveToasterId = "inbox-live"
export const liveToastDuration = 8000

type Context = { panelOpen: boolean; resync: boolean }

export type LiveToastGate = { seen: number; fetch: boolean }

export type LiveToastPlan = {
  seen: number
  items: InboxItem[]
  overflow: number
}

export function gateLiveToasts(
  newest: number,
  seen: number | null,
  { panelOpen, resync }: Context
): LiveToastGate {
  if (seen === null) return { seen: newest, fetch: false }
  if (panelOpen || resync) return { seen: Math.max(seen, newest), fetch: false }
  return { seen, fetch: newest > seen }
}

export function planLiveToasts(
  items: readonly InboxItem[],
  seen: number
): LiveToastPlan {
  const next = items.reduce((top, item) => Math.max(top, item.seq), seen)
  const fresh = items
    .filter((item) => item.seq > seen && item.reason !== null)
    .sort((a, b) => a.seq - b.seq)
  if (fresh.length > maxLiveToasts) {
    return { seen: next, items: [], overflow: fresh.length }
  }
  return { seen: next, items: fresh, overflow: 0 }
}
