import type { components } from "@/api/schema.gen"
import { groupThreads, type Thread } from "@/comments/threads"
import type { Comment } from "@/comments/queries"
import type { VersionMeta } from "@/history/use-version-history"
import { isActive, type Revision } from "@/revisions/queries"

type ImportRecord = components["schemas"]["ImportRecord"]

export type UnverifiedPanel = {
  panelId: string
  title: string
  reason: string
}

export type ImportedDashboard = {
  record: ImportRecord
  unverified: readonly UnverifiedPanel[]
}

export type TimelineEntry =
  | {
      kind: "thread"
      key: string
      at: string
      panelId: string | null
      thread: Thread
      request: Revision | null
      askedAgent: boolean
    }
  | {
      kind: "request"
      key: string
      at: string
      panelId: string | null
      request: Revision
    }
  | {
      kind: "running"
      key: string
      at: string
      panelId: string | null
      request: Revision
    }
  | {
      kind: "version"
      key: string
      at: string
      panelId: null
      version: VersionMeta
    }
  | {
      kind: "import"
      key: "import"
      at: string
      panelId: null
      record: ImportRecord
      unverified: readonly UnverifiedPanel[]
    }

type StepStamp = Pick<components["schemas"]["AgentStep"], "attempt" | "at">

export function currentStep(steps: readonly StepStamp[]): number {
  const last = steps.at(-1)
  if (last === undefined) return 0
  let first = steps.length - 1
  while (first > 0) {
    const step = steps[first - 1]
    const next = steps[first]
    if (
      step === undefined ||
      next === undefined ||
      step.attempt !== last.attempt ||
      timeOf(step.at) > timeOf(next.at)
    ) {
      break
    }
    first -= 1
  }
  return steps.length - first
}

export type TimelineShow = "everything" | "open" | "changes"

const kindOrder: Record<TimelineEntry["kind"], number> = {
  thread: 0,
  request: 1,
  running: 2,
  version: 3,
  import: 4,
}

const sameSendWindowMs = 2000

function timeOf(iso: string): number {
  const time = Date.parse(iso)
  return Number.isNaN(time) ? Number.NEGATIVE_INFINITY : time
}

function compareNumbers(a: number, b: number): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function compareText(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function compareTies(a: TimelineEntry, b: TimelineEntry): number {
  const byKind = kindOrder[a.kind] - kindOrder[b.kind]
  if (byKind !== 0) return byKind
  if (a.kind === "version" && b.kind === "version") {
    const byNumber = a.version.version - b.version.version
    if (byNumber !== 0) return byNumber
  }
  return compareText(a.key, b.key)
}

function sortEntries(entries: readonly TimelineEntry[]): TimelineEntry[] {
  return entries
    .map((entry) => ({ entry, time: timeOf(entry.at) }))
    .sort(
      (a, b) => compareNumbers(a.time, b.time) || compareTies(a.entry, b.entry)
    )
    .map(({ entry }) => entry)
}

function newestRequest(requests: readonly Revision[]): Revision | null {
  let newest: Revision | null = null
  let newestTime = Number.NEGATIVE_INFINITY
  for (const request of requests) {
    const time = timeOf(request.created_at)
    if (
      newest === null ||
      time > newestTime ||
      (time === newestTime && request.id > newest.id)
    ) {
      newest = request
      newestTime = time
    }
  }
  return newest
}

function askedAgentWithComment(
  root: Comment,
  requests: readonly Revision[]
): boolean {
  const rootTime = timeOf(root.created_at)
  return requests.some(
    (request) =>
      request.requested_by.kind === root.author.kind &&
      request.requested_by.id === root.author.id &&
      Math.abs(timeOf(request.created_at) - rootTime) <= sameSendWindowMs
  )
}

function runningAt(request: Revision): string {
  const raw = request.started_at ?? request.created_at
  const start = Date.parse(raw)
  return Number.isNaN(start) ? raw : new Date(start + 1).toISOString()
}

export function buildTimeline(input: {
  comments: readonly Comment[]
  requests: readonly Revision[]
  versions: readonly VersionMeta[]
  imported?: ImportedDashboard | null
}): TimelineEntry[] {
  const entries: TimelineEntry[] = []

  const byComment = new Map<string, Revision[]>()
  for (const request of input.requests) {
    if (request.comment_id === null) continue
    const list = byComment.get(request.comment_id)
    if (list === undefined) byComment.set(request.comment_id, [request])
    else list.push(request)
  }

  const threads = groupThreads(input.comments)
  for (const thread of threads) {
    const requests = byComment.get(thread.root.id) ?? []
    entries.push({
      kind: "thread",
      key: `thread:${thread.root.id}`,
      at: thread.root.created_at,
      panelId: thread.root.panel_id,
      thread,
      request: newestRequest(requests),
      askedAgent: askedAgentWithComment(thread.root, requests),
    })
  }

  const threadIds = new Set(threads.map((thread) => thread.root.id))
  for (const request of input.requests) {
    const hasThread =
      request.comment_id !== null && threadIds.has(request.comment_id)
    if (request.source !== "observe" && !hasThread) {
      entries.push({
        kind: "request",
        key: `request:${request.id}`,
        at: request.created_at,
        panelId: request.panel_id,
        request,
      })
    }
    if (isActive(request.status)) {
      entries.push({
        kind: "running",
        key: `running:${request.id}`,
        at: runningAt(request),
        panelId: request.panel_id,
        request,
      })
    }
  }

  for (const version of input.versions) {
    entries.push({
      kind: "version",
      key: `version:${version.version}`,
      at: version.created_at,
      panelId: null,
      version,
    })
  }

  if (input.imported) {
    entries.push({
      kind: "import",
      key: "import",
      at: input.imported.record.created_at,
      panelId: null,
      record: input.imported.record,
      unverified: input.imported.unverified,
    })
  }

  return sortEntries(entries)
}

function matchesShow(entry: TimelineEntry, show: TimelineShow): boolean {
  if (entry.kind === "import") return show === "everything"
  switch (show) {
    case "everything":
      return true
    case "open":
      return entry.kind === "thread" && entry.thread.root.status === "open"
    case "changes":
      return entry.kind !== "thread" || entry.request !== null
  }
}

function matchesPanel(entry: TimelineEntry, panelId: string | null): boolean {
  if (panelId === null) return true
  if (entry.kind === "import") {
    return entry.unverified.some((panel) => panel.panelId === panelId)
  }
  return entry.panelId === panelId
}

export function filterTimeline(
  entries: readonly TimelineEntry[],
  show: TimelineShow,
  panelId: string | null
): TimelineEntry[] {
  return entries.filter(
    (entry) => matchesShow(entry, show) && matchesPanel(entry, panelId)
  )
}
