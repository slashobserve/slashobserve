import type { AuditWindowKey } from "@/api/keys"
import { isTimeValue, requestZone, resolveTime } from "@/lib/time"

export type AuditView = {
  q: string
  from: string
  to: string
  event: string | null
}

export type AuditSearch = {
  q?: string
  from?: string
  to?: string
  event?: string
}

export const defaultFrom = "now-24h"
export const defaultTo = "now"
const maxWindowDays = 366
const maxQueryLength = 65_536
const dayMs = 86_400_000
const eventId =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

function text(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

export function readAuditSearch(raw: Record<string, unknown>): AuditSearch {
  const q = text(raw.q)
  const from = text(raw.from)
  const to = text(raw.to)
  const event = text(raw.event)
  return {
    q: q !== undefined && q.length <= maxQueryLength ? q : undefined,
    from: from !== undefined && isTimeValue(from) ? from : undefined,
    to: to !== undefined && isTimeValue(to) ? to : undefined,
    event: event !== undefined && eventId.test(event) ? event : undefined,
  }
}

export function auditViewOf(search: AuditSearch): AuditView {
  return {
    q: search.q ?? "",
    from: search.from ?? defaultFrom,
    to: search.to ?? defaultTo,
    event: search.event ?? null,
  }
}

export function auditSearchOf(view: AuditView): AuditSearch {
  return {
    q: view.q === "" ? undefined : view.q,
    from: view.from === defaultFrom ? undefined : view.from,
    to: view.to === defaultTo ? undefined : view.to,
    event: view.event ?? undefined,
  }
}

export function followsNow(view: Pick<AuditView, "to">): boolean {
  return view.to === "now"
}

export type AuditWindow =
  | { since: string; until: string; problem?: never }
  | { problem: string; since?: never; until?: never }

export function auditWindow(
  from: string,
  to: string,
  now: number,
  zone: string | undefined = requestZone("browser")
): AuditWindow {
  const since = resolveTime(from, now, false, { zone })
  const roundedEnd = to.includes("/")
  const until =
    resolveTime(to, now, roundedEnd, { zone }) + (roundedEnd ? 1 : 0)
  if (Number.isNaN(since) || Number.isNaN(until)) {
    return { problem: "The time range could not be read." }
  }
  if (until <= since) {
    return { problem: "The time range ends before it starts." }
  }
  if (until - since > maxWindowDays * dayMs) {
    return {
      problem: `The audit log searches at most ${maxWindowDays} days at a time. Choose a shorter time range.`,
    }
  }
  return {
    since: new Date(since).toISOString(),
    until: new Date(until).toISOString(),
  }
}

function spanOf(window: AuditWindowKey): number {
  return Date.parse(window.until) - Date.parse(window.since)
}

export function sameSearchLater(
  previous: AuditWindowKey,
  next: AuditWindowKey
): boolean {
  return (
    previous.q === next.q &&
    spanOf(previous) === spanOf(next) &&
    Date.parse(next.until) >= Date.parse(previous.until)
  )
}
