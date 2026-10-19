import { groupThreads, type Thread } from "@/comments/threads"
import type { Comment } from "@/comments/queries"
import type { VersionMeta } from "@/history/use-version-history"
import { isActive, type Revision } from "@/revisions/queries"

export type TimelineEntry =
  | {
      kind: "thread"
      key: string
      at: string
      panelId: string | null
      thread: Thread
      request: Revision | null
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

export type TimelineShow = "everything" | "open" | "changes"

const kindOrder: Record<TimelineEntry["kind"], number> = {
  thread: 0,
  request: 1,
  running: 2,
  version: 3,
}

function compareText(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0
}

function compareEntries(a: TimelineEntry, b: TimelineEntry): number {
  const byTime = Date.parse(a.at) - Date.parse(b.at)
  if (byTime !== 0) return byTime
  const byKind = kindOrder[a.kind] - kindOrder[b.kind]
  if (byKind !== 0) return byKind
  return compareText(a.key, b.key)
}

function newestRequest(requests: readonly Revision[]): Revision | null {
  let newest: Revision | null = null
  for (const request of requests) {
    if (
      newest === null ||
      Date.parse(request.created_at) > Date.parse(newest.created_at) ||
      (request.created_at === newest.created_at && request.id > newest.id)
    ) {
      newest = request
    }
  }
  return newest
}

function runningAt(request: Revision): string {
  const start = Date.parse(request.started_at ?? request.created_at)
  return new Date(start + 1).toISOString()
}

export function buildTimeline(input: {
  comments: readonly Comment[]
  requests: readonly Revision[]
  versions: readonly VersionMeta[]
}): TimelineEntry[] {
  const entries: TimelineEntry[] = []

  const byComment = new Map<string, Revision[]>()
  for (const request of input.requests) {
    if (request.comment_id === null) continue
    const list = byComment.get(request.comment_id)
    if (list === undefined) byComment.set(request.comment_id, [request])
    else list.push(request)
  }

  for (const thread of groupThreads(input.comments)) {
    entries.push({
      kind: "thread",
      key: `thread:${thread.root.id}`,
      at: thread.root.created_at,
      panelId: thread.root.panel_id,
      thread,
      request: newestRequest(byComment.get(thread.root.id) ?? []),
    })
  }

  for (const request of input.requests) {
    if (request.source !== "observe" && request.comment_id === null) {
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

  return entries.sort(compareEntries)
}

function matchesShow(entry: TimelineEntry, show: TimelineShow): boolean {
  switch (show) {
    case "everything":
      return true
    case "open":
      return entry.kind === "thread" && entry.thread.root.status === "open"
    case "changes":
      return entry.kind !== "thread"
  }
}

export function filterTimeline(
  entries: readonly TimelineEntry[],
  show: TimelineShow,
  panelId: string | null
): TimelineEntry[] {
  return entries.filter(
    (entry) =>
      matchesShow(entry, show) &&
      (panelId === null || entry.panelId === panelId)
  )
}
