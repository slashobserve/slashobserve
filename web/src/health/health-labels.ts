import type { components } from "@/api/schema.gen"
import type { StatusTone } from "@/components/status-label"

export type HealthKind = components["schemas"]["HealthProblemKind"]
export type HealthProblem = components["schemas"]["HealthProblem"]
export type HealthItem = components["schemas"]["HealthItem"]
export type HealthJob = components["schemas"]["HealthJob"]
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

export function isHealthKind(value: unknown): value is HealthKind {
  return healthKinds.includes(value as HealthKind)
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

export function canRepair(item: HealthItem): boolean {
  return item.can_edit && item.problems.some((p) => repairableKinds.has(p.kind))
}

export function repairReason(
  item: HealthItem,
  agentAvailable: boolean
): string | null {
  if (!item.can_edit) return "You cannot edit this dashboard"
  if (!canRepair(item)) return "Nothing here the agent can repair"
  if (!agentAvailable) return "No agent is configured"
  return null
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

export type JobLine = {
  tone: StatusTone
  label: string
  at: string | null
}

export function jobLine(job: HealthJob): JobLine {
  if (job.running) {
    return { tone: "running", label: "Checking dashboards now", at: null }
  }
  switch (job.last_outcome) {
    case "succeeded":
      return {
        tone: "ok",
        label: "Last run finished",
        at: job.last_finished_at,
      }
    case "failed":
      return {
        tone: "critical",
        label: "Last run failed",
        at: job.last_finished_at,
      }
    case "interrupted":
      return {
        tone: "warning",
        label: "Last run was interrupted",
        at: job.last_finished_at ?? job.last_started_at,
      }
    default:
      return { tone: "none", label: "No run has finished yet", at: null }
  }
}

export type RunRequest = { previousStart: string | null; at: number }

export function pollHealth(
  job: HealthJob | undefined,
  run: RunRequest | null,
  now: number
): boolean {
  if (job === undefined) return false
  if (job.running) return true
  if (run === null || job.last_started_at !== run.previousStart) return false
  return now - run.at < runStartWaitMs
}
