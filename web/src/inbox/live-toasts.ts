import type { InboxItem } from "./inbox-item"

const maxLiveToasts = 3
export const liveToasterId = "inbox-live"
export const liveToastDuration = 8000

export type LiveToastPlan = {
  seen: number | null
  items: InboxItem[]
  overflow: number
}

type Context = { panelOpen: boolean; resync: boolean }

function newest(items: readonly InboxItem[], floor: number): number {
  return items.reduce((top, item) => Math.max(top, item.seq), floor)
}

export function planLiveToasts(
  items: readonly InboxItem[],
  seen: number | null,
  { panelOpen, resync }: Context
): LiveToastPlan {
  if (seen === null) {
    return { seen: newest(items, 0), items: [], overflow: 0 }
  }
  const next = newest(items, seen)
  if (panelOpen || resync) return { seen: next, items: [], overflow: 0 }
  const fresh = items
    .filter((item) => item.seq > seen && item.reason !== null)
    .sort((a, b) => a.seq - b.seq)
  if (fresh.length > maxLiveToasts) {
    return { seen: next, items: [], overflow: fresh.length }
  }
  return { seen: next, items: fresh, overflow: 0 }
}
