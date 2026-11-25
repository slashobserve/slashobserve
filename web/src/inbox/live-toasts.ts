import type { InboxItem } from "./inbox-item"

const maxLiveToasts = 3
export const liveToasterId = "inbox-live"
export const liveToastDuration = 8000

type Context = { panelOpen: boolean; hidden: boolean }

export type LiveToastGate = { seen: number; fresh: boolean }

export type LiveToastPlan = {
  seen: number
  items: InboxItem[]
  overflow: number
}

export function gateLiveToasts(
  newest: number,
  seen: number | null,
  { panelOpen, hidden }: Context
): LiveToastGate {
  if (seen === null) return { seen: newest, fresh: false }
  if (panelOpen) return { seen: Math.max(seen, newest), fresh: false }
  return { seen, fresh: !hidden && newest > seen }
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
