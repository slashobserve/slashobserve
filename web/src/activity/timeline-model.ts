import type { components } from "@/api/schema.gen"
import type { Annotation } from "@/annotations/queries"
import type { ThreadWithMessages } from "@/comments/thread-queries"
import type { Finding } from "@/findings/queries"
import type { VersionMeta } from "@/history/use-version-history"
import type { RequestSummary } from "@/observe/queries"
import type { ProposalSummary } from "@/proposals/queries"
import { requestOrigin } from "@/observe/request-origin"
import { isActive, type Revision } from "@/revisions/queries"

type ImportRecord = components["schemas"]["ImportRecord"]

type UnverifiedPanel = {
  panelId: string
  title: string
  reason: string
}

export type ImportedDashboard = {
  record: ImportRecord
  unverified: readonly UnverifiedPanel[]
}

export type RunningRequest = Pick<
  RequestSummary,
  "id" | "status" | "dashboard_id" | "panel_id" | "created_at" | "started_at"
>

export type TimelineEntry =
  | {
      kind: "thread"
      key: string
      at: string
      panelId: string | null
      thread: ThreadWithMessages
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
      request: RunningRequest
    }
  | {
      kind: "version"
      key: string
      at: string
      panelId: null
      version: VersionMeta
      proposal: ProposalSummary | null
    }
  | {
      kind: "proposal"
      key: string
      at: string
      panelId: null
      proposal: ProposalSummary
    }
  | {
      kind: "annotation"
      key: string
      at: string
      panelId: string | null
      annotation: Annotation
    }
  | {
      kind: "import"
      key: "import"
      at: string
      panelId: null
      record: ImportRecord
      unverified: readonly UnverifiedPanel[]
    }
  | {
      kind: "finding"
      key: string
      at: string
      panelId: null
      finding: Finding
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
  proposal: 4,
  annotation: 5,
  import: 6,
  finding: 7,
}

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

function runningAt(request: RunningRequest): string {
  const raw = request.started_at ?? request.created_at
  const start = Date.parse(raw)
  return Number.isNaN(start) ? raw : new Date(start + 1).toISOString()
}

function runningEntry(request: RunningRequest): TimelineEntry {
  return {
    kind: "running",
    key: `running:${request.id}`,
    at: runningAt(request),
    panelId: request.panel_id,
    request,
  }
}

export function buildTimeline(input: {
  threads: readonly ThreadWithMessages[]
  requests: readonly Revision[]
  versions: readonly VersionMeta[]
  imported?: ImportedDashboard | null
  investigations?: readonly RunningRequest[]
  findings?: readonly Finding[] | undefined
  proposals?: readonly ProposalSummary[]
  annotations?: readonly Annotation[] | undefined
}): TimelineEntry[] {
  const entries: TimelineEntry[] = []

  for (const thread of input.threads) {
    if (thread.messages.length === 0) continue
    entries.push({
      kind: "thread",
      key: `thread:${thread.thread.id}`,
      at: thread.thread.created_at,
      panelId: thread.thread.panel_id,
      thread,
    })
  }

  const threadIds = new Set(input.threads.map((thread) => thread.thread.id))
  for (const request of input.requests) {
    const inThread =
      request.comment_id !== null && threadIds.has(request.comment_id)
    if (inThread) continue
    if (requestOrigin(request) !== "observe") {
      entries.push({
        kind: "request",
        key: `request:${request.id}`,
        at: request.created_at,
        panelId: request.panel_id,
        request,
      })
    }
    if (isActive(request.status)) entries.push(runningEntry(request))
  }

  for (const request of input.investigations ?? []) {
    if (isActive(request.status)) entries.push(runningEntry(request))
  }

  for (const finding of input.findings ?? []) {
    entries.push({
      kind: "finding",
      key: `finding:${finding.id}`,
      at: finding.created_at,
      panelId: null,
      finding,
    })
  }

  const merged = new Map<string, ProposalSummary>()
  for (const proposal of input.proposals ?? []) {
    if (proposal.state === "merged") merged.set(proposal.id, proposal)
    if (proposal.state === "open") {
      entries.push({
        kind: "proposal",
        key: `proposal:${proposal.id}`,
        at: proposal.created_at,
        panelId: null,
        proposal,
      })
    }
  }

  for (const annotation of input.annotations ?? []) {
    entries.push({
      kind: "annotation",
      key: `annotation:${annotation.id}`,
      at: annotation.created_at,
      panelId: annotation.panel_id,
      annotation,
    })
  }

  for (const version of input.versions) {
    entries.push({
      kind: "version",
      key: `version:${version.version}`,
      at: version.created_at,
      panelId: null,
      version,
      proposal:
        version.merged_proposal_id === null
          ? null
          : (merged.get(version.merged_proposal_id) ?? null),
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

export function showsVersions(
  show: TimelineShow,
  panelId: string | null
): boolean {
  return show !== "open" && panelId === null
}

export function firstNewVersion(
  entries: readonly TimelineEntry[],
  known: ReadonlySet<number>
): number | null {
  for (const entry of entries) {
    if (entry.kind === "version" && !known.has(entry.version.version)) {
      return entry.version.version
    }
  }
  return null
}

function matchesShow(entry: TimelineEntry, show: TimelineShow): boolean {
  if (entry.kind === "import") return show === "everything"
  switch (show) {
    case "everything":
      return true
    case "open":
      return (
        entry.kind === "proposal" ||
        (entry.kind === "thread" && entry.thread.thread.status === "open")
      )
    case "changes":
      return entry.kind !== "thread" || entry.thread.thread.revision !== null
  }
}

function matchesPanel(entry: TimelineEntry, panelId: string | null): boolean {
  if (
    panelId === null ||
    entry.kind === "finding" ||
    entry.kind === "proposal"
  ) {
    return true
  }
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
