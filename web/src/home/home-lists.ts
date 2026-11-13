import type { InboxItem } from "@/inbox/inbox-item"
import type { RequestSummary } from "@/observe/queries"

import type { RecentEntry, RecentKind } from "./recent"

export type PickUpEntry = RecentEntry & { kind: RecentKind }

function newestFirst(a: RequestSummary, b: RequestSummary): number {
  return Date.parse(b.created_at) - Date.parse(a.created_at)
}

export function newestChats(
  requests: readonly RequestSummary[],
  limit: number
): RequestSummary[] {
  return requests.toSorted(newestFirst).slice(0, limit)
}

export function activeRuns(
  requests: readonly RequestSummary[]
): RequestSummary[] {
  return requests
    .filter(
      (request) =>
        request.state === "queued" ||
        request.state === "running" ||
        request.state === "canceling"
    )
    .toSorted(newestFirst)
}

export function needsYou(
  items: readonly InboxItem[],
  limit: number
): InboxItem[] {
  return items.filter((item) => item.reason !== null).slice(0, limit)
}

export function createdBy<T extends { created_by: { id: string } }>(
  items: readonly T[],
  personId: string,
  limit: number
): T[] {
  return items.filter((item) => item.created_by.id === personId).slice(0, limit)
}

export function pickUp(
  dashboards: readonly RecentEntry[],
  observations: readonly RecentEntry[],
  limit: number
): PickUpEntry[] {
  const tagged: PickUpEntry[] = [
    ...dashboards.map((entry): PickUpEntry => ({
      ...entry,
      kind: "dashboard",
    })),
    ...observations.map((entry): PickUpEntry => ({
      ...entry,
      kind: "observation",
    })),
  ]
  return tagged
    .toSorted((a, b) => Date.parse(b.viewed_at) - Date.parse(a.viewed_at))
    .slice(0, limit)
}
