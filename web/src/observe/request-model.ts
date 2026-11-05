import type { ObserveRequest } from "@/observe/queries"

export type Step = ObserveRequest["steps"][number]

export type ToolCount = { tool: string; count: number }

export type TimelinePhase = {
  key: string
  phase: Step["phase"]
  steps: Step[]
  tools: ToolCount[]
  errors: number
  durationMs: number
}

export type RetryAction =
  | { kind: "create" }
  | { kind: "revise"; dashboardId: string }
  | {
      kind: "investigate"
      dashboardId?: string
      panelId?: string
      from?: string
      to?: string
    }
  | { kind: "none"; dashboardId: string | null }

function continuesRun(run: TimelinePhase | undefined, step: Step): boolean {
  if (run === undefined || step.phase === "repair") return false
  return run.phase === step.phase && run.steps[0].attempt === step.attempt
}

export function timelinePhases(steps: readonly Step[]): TimelinePhase[] {
  const runs: TimelinePhase[] = []
  steps.forEach((step, index) => {
    let run = runs.at(-1)
    if (run === undefined || !continuesRun(run, step)) {
      run = {
        key: `${step.attempt}:${index}`,
        phase: step.phase,
        steps: [],
        tools: [],
        errors: 0,
        durationMs: 0,
      }
      runs.push(run)
    }
    run.steps.push(step)
    const counted = run.tools.find((entry) => entry.tool === step.tool)
    if (counted) counted.count += 1
    else run.tools.push({ tool: step.tool, count: 1 })
    if (step.status === "error") run.errors += 1
    run.durationMs += step.duration_ms
  })
  return runs
}

type RetrySource = Pick<ObserveRequest, "source" | "dashboard_id"> &
  Partial<Pick<ObserveRequest, "panel_id" | "from" | "to">>

function investigateAgain(request: RetrySource): RetryAction {
  const window =
    request.from != null && request.to != null
      ? { from: request.from, to: request.to }
      : {}
  return {
    kind: "investigate",
    ...(request.dashboard_id === null
      ? {}
      : { dashboardId: request.dashboard_id }),
    ...(request.panel_id == null ? {} : { panelId: request.panel_id }),
    ...window,
  }
}

export function retryAction(request: RetrySource): RetryAction {
  if (request.source === "observe") return { kind: "create" }
  if (request.source === "investigate") return investigateAgain(request)
  if (request.source === "prompt" && request.dashboard_id !== null) {
    return { kind: "revise", dashboardId: request.dashboard_id }
  }
  return { kind: "none", dashboardId: request.dashboard_id }
}
