import type { components } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"
import { sentence } from "@/observe/queries"

export type HealthKind = components["schemas"]["HealthProblemKind"]
export type HealthProblem = components["schemas"]["HealthProblem"]
export type HealthItem = components["schemas"]["HealthItem"]
export type HealthJob = components["schemas"]["HealthJob"]
export type HealthSummary = components["schemas"]["HealthSummary"]
type RepairsQueued = components["schemas"]["RepairsQueued"]

export const healthKinds: readonly HealthKind[] = [
  "query_failed",
  "variable_failed",
  "no_data",
  "duplicate_title",
  "unopened",
]

export const kindLabels: Record<HealthKind, string> = {
  query_failed: "Query fails",
  variable_failed: "Variable resolves to nothing",
  no_data: "No data",
  duplicate_title: "Duplicate title",
  unopened: "Not opened in 90 days",
}

export const kindTones: Record<HealthKind, StatusTone> = {
  query_failed: "critical",
  variable_failed: "critical",
  no_data: "warning",
  duplicate_title: "none",
  unopened: "none",
}

const repairableKinds: ReadonlySet<HealthKind> = new Set([
  "query_failed",
  "variable_failed",
  "no_data",
])

export const runStartWaitMs = 60_000

export const importedFilter = "imported"

export const importedLabel = "Imported, not verified"

export type HealthFilter = HealthKind | typeof importedFilter

export function isHealthFilter(value: unknown): value is HealthFilter {
  return value === importedFilter || healthKinds.includes(value as HealthKind)
}

export type ProblemFilter = "any" | HealthKind | typeof importedFilter

export function isProblemFilter(value: unknown): value is ProblemFilter {
  return value === "any" || isHealthFilter(value)
}

export const problemFilterLabels: Record<ProblemFilter, string> = {
  any: "Any problem",
  ...kindLabels,
  [importedFilter]: importedLabel,
}

export function sortProblems(problems: readonly HealthProblem[]) {
  return problems.toSorted(
    (a, b) =>
      healthKinds.indexOf(a.kind) - healthKinds.indexOf(b.kind) ||
      problemSubject(a).localeCompare(problemSubject(b))
  )
}

export function problemSubject(problem: HealthProblem): string {
  if (problem.variable !== null) return `Variable ${problem.variable}`
  if (problem.panel_title !== null) return problem.panel_title
  if (problem.panel_id !== null) return `Panel ${problem.panel_id}`
  return ""
}

export function dashboardProblems(item: HealthItem): HealthProblem[] {
  return item.problems.filter((p) => p.kind !== "unopened")
}

export function panelProblems(
  item: HealthItem,
  drawnVersion: number
): ReadonlyMap<string, readonly HealthProblem[]> {
  const byPanel = new Map<string, HealthProblem[]>()
  if (item.checked_version !== drawnVersion) return byPanel
  for (const problem of item.problems) {
    if (problem.panel_id === null) continue
    const list = byPanel.get(problem.panel_id) ?? []
    list.push(problem)
    byPanel.set(problem.panel_id, list)
  }
  return byPanel
}

export function problemCountText(count: number): string {
  return `${count} ${count === 1 ? "problem" : "problems"}`
}

export function worstTone(problems: readonly HealthProblem[]): StatusTone {
  const [worst] = sortProblems(problems)
  return worst === undefined ? "none" : kindTones[worst.kind]
}

export function canRepair(item: HealthItem): boolean {
  return item.can_edit && item.problems.some((p) => repairableKinds.has(p.kind))
}

export function repairLabel(problems: readonly HealthProblem[]): string {
  return problems.filter((p) => repairableKinds.has(p.kind)).length > 1
    ? "Repair all"
    : "Repair"
}

export type ProblemGroup = {
  kind: HealthKind
  message: string | null
  subjects: string[]
}

export function groupProblems(
  problems: readonly HealthProblem[]
): ProblemGroup[] {
  const groups = new Map<string, ProblemGroup>()
  for (const problem of sortProblems(problems)) {
    const key = JSON.stringify([problem.kind, problem.message])
    const group = groups.get(key) ?? {
      kind: problem.kind,
      message: problem.message,
      subjects: [],
    }
    groups.set(key, group)
    const subject = problemSubject(problem)
    if (subject !== "") group.subjects.push(subject)
  }
  return [...groups.values()]
}

export function healthCellText(
  item: HealthItem
): { group: ProblemGroup; more: number } | null {
  const [group, ...rest] = groupProblems(item.problems)
  return group === undefined ? null : { group, more: rest.length }
}

export function subjectNoun(group: ProblemGroup): string {
  return group.kind === "variable_failed" ? "variables" : "panels"
}

export function subjectSummary(group: ProblemGroup): string | null {
  const [only, ...rest] = group.subjects
  if (only === undefined) return null
  if (rest.length === 0) return only
  return `${group.subjects.length} ${subjectNoun(group)}`
}

export function problemText(
  problem: Pick<HealthProblem, "kind" | "message">
): string | null {
  if (problem.kind === "no_data") return "No data in the last 7 days."
  return problem.message === null ? null : sentence(problem.message)
}

const noAgentReason = "No agent is configured"

export type RepairAction =
  | { kind: "queued" }
  | { kind: "running" }
  | { kind: "repaired"; version: number }
  | {
      kind: "failed"
      error: string | null
      canRetry: boolean
      disabledReason: string | null
    }
  | { kind: "repair"; disabledReason: string | null }
  | { kind: "open" }

export function repairAction(
  item: HealthItem,
  agentAvailable: boolean
): RepairAction {
  const disabledReason = agentAvailable ? null : noAgentReason
  const repair = item.repair
  switch (repair?.outcome) {
    case "queued":
      return { kind: "queued" }
    case "running":
      return { kind: "running" }
    case "failed":
      return {
        kind: "failed",
        error: repair.error === null ? null : sentence(repair.error),
        canRetry: canRepair(item),
        disabledReason,
      }
    case "repaired":
      if (repair.version !== null) {
        return { kind: "repaired", version: repair.version }
      }
  }
  return canRepair(item) ? { kind: "repair", disabledReason } : { kind: "open" }
}

export function repairStatus(action: RepairAction): SummaryLabel | null {
  switch (action.kind) {
    case "queued":
      return { tone: "running", label: "Repair queued" }
    case "running":
      return { tone: "running", label: "Repairing" }
    case "repaired":
      return { tone: "ok", label: "Repaired" }
    case "failed":
      return { tone: "critical", label: "Repair failed" }
    default:
      return null
  }
}

function repairActive(item: HealthItem): boolean {
  return item.repair?.outcome === "queued" || item.repair?.outcome === "running"
}

export type SummaryLabel = { tone: StatusTone; label: string }

export function summaryLabels(summary: HealthSummary): SummaryLabel[] {
  const labels: [number, StatusTone, string][] = [
    [summary.failing, "critical", `${summary.failing} failing`],
    [summary.no_data, "warning", `${summary.no_data} without data`],
    [summary.unopened, "none", `${summary.unopened} not opened in 90 days`],
    [
      summary.duplicate_titles,
      "none",
      `${summary.duplicate_titles} duplicate ${summary.duplicate_titles === 1 ? "title" : "titles"}`,
    ],
  ]
  return labels
    .filter(([count]) => count > 0)
    .map(([, tone, label]) => ({ tone, label }))
}

const skipTexts: Record<RepairsQueued["skipped"][number]["reason"], string> = {
  active: "A revision is already queued or running on this dashboard.",
  nothing_to_repair: "The last check found nothing to repair.",
  not_found: "The dashboard no longer exists.",
  forbidden_role: "You cannot edit this dashboard.",
  queue_full:
    "You already have 20 repairs queued or running. Try again when some finish.",
}

export function repairOutcomeText(outcome: RepairsQueued): string {
  if (outcome.queued.length > 0) {
    return "Repair queued. The agent saves it as a revision to review on the dashboard."
  }
  const skipped = outcome.skipped[0]
  return skipped === undefined
    ? "No repair queued."
    : `No repair queued. ${skipTexts[skipped.reason]}`
}

const kindCounts: Record<HealthKind, keyof HealthSummary> = {
  query_failed: "query_failed",
  variable_failed: "variable_failed",
  no_data: "no_data",
  duplicate_title: "duplicate_titles",
  unopened: "unopened",
}

export function dashboardCount(
  summary: HealthSummary,
  kind: HealthKind | null
): string {
  const count = summary[kind === null ? "dashboards" : kindCounts[kind]]
  return `${count} ${count === 1 ? "dashboard" : "dashboards"}`
}

export function lastRunProblem(job: HealthJob): SummaryLabel | null {
  if (job.running) return null
  switch (job.last_outcome) {
    case "failed":
      return { tone: "critical", label: "Last run failed" }
    case "interrupted":
      return { tone: "warning", label: "Last run was interrupted" }
    default:
      return null
  }
}

export type RunRequest = { previousStart: string | null; at: number }

export function pollHealth(
  job: HealthJob | undefined,
  items: readonly HealthItem[],
  run: RunRequest | null,
  now: number
): boolean {
  if (items.some(repairActive)) return true
  if (job === undefined) return false
  if (job.running) return true
  if (run === null || job.last_started_at !== run.previousStart) return false
  return now - run.at < runStartWaitMs
}
