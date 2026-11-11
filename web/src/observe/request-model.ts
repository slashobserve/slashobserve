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
