import type { components } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"
import { importStatus } from "@/import/record-summary"
import type { InboxItem } from "@/inbox/inbox-item"
import {
  markOfAuthor,
  observeAgentMark,
  type AuthorMarkModel,
} from "@/lib/author"
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

export type WorkingRun = components["schemas"]["WorkingRuns"]["items"][number]
type ImportCounts = components["schemas"]["ImportCounts"]

export type WorkingTarget =
  | { kind: "chat"; id: string }
  | { kind: "dashboard"; id: string; title: string }
  | { kind: "observation"; id: string }

export function agentMark(agent: WorkingRun["agent"]): AuthorMarkModel {
  if (agent.kind === "builtin") return observeAgentMark
  return markOfAuthor({
    kind: "service_account",
    id: agent.name,
    name: agent.name,
  })
}

export function workingTitle(run: WorkingRun): string {
  return (
    (run.mine ? run.title : null) ??
    run.dashboard?.title ??
    run.observation?.title ??
    "something of yours"
  )
}

export function workingPlaces(run: WorkingRun): string[] {
  const named = run.mine ? null : (run.dashboard ?? run.observation)
  const places: string[] = []
  if (run.observation !== null && run.observation !== named) {
    places.push(`in ${run.observation.title}`)
  }
  if (run.dashboard !== null && run.dashboard !== named) {
    places.push(`on ${run.dashboard.title}`)
  }
  return places
}

export function workingTarget(run: WorkingRun): WorkingTarget | null {
  if (run.mine) return { kind: "chat", id: run.id }
  if (run.dashboard !== null) {
    return { kind: "dashboard", ...run.dashboard }
  }
  if (run.observation !== null) {
    return { kind: "observation", id: run.observation.id }
  }
  return null
}

export function elapsedText(ms: number): string {
  const seconds = Math.max(0, Math.floor(ms / 1000))
  if (seconds < 60) return `${seconds} s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes} min`
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  return rest === 0 ? `${hours} h` : `${hours} h ${rest} min`
}

export function runningText(run: WorkingRun, now: number): string {
  if (run.started_at === null) return "Queued"
  return `Running, ${elapsedText(now - Date.parse(run.started_at))}`
}

export function messageCountText(count: number): string | null {
  if (count <= 0) return null
  return count === 1 ? "1 message" : `${count} messages`
}

export function importMeta(counts: ImportCounts): {
  status: { tone: StatusTone; text: string } | null
  counts: string
} {
  const panels = `${counts.total} ${counts.total === 1 ? "panel" : "panels"}`
  const placeholders =
    counts.placeholder === 0
      ? ""
      : `, ${counts.placeholder} ${counts.placeholder === 1 ? "placeholder" : "placeholders"}`
  const placeholdersOnly = counts.unverified === 0 && counts.placeholder > 0
  return {
    status: placeholdersOnly ? null : importStatus(counts),
    counts: panels + placeholders,
  }
}
