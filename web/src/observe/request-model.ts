import type { ObserveRequest } from "@/observe/queries"

export type Step = ObserveRequest["steps"][number]

export type StepGroup = { step: Step; count: number }

export type RetryAction =
  | { kind: "create" }
  | { kind: "revise"; dashboardId: string }
  | { kind: "none"; dashboardId: string | null }

export function groupSteps(steps: Step[]): StepGroup[] {
  const groups: StepGroup[] = []
  for (const step of steps) {
    const last = groups.at(-1)
    if (
      last !== undefined &&
      step.status === "ok" &&
      step.phase !== "repair" &&
      last.step.status === "ok" &&
      last.step.phase === step.phase &&
      last.step.attempt === step.attempt
    ) {
      last.step = step
      last.count += 1
    } else {
      groups.push({ step, count: 1 })
    }
  }
  return groups
}

export function retryAction(
  request: Pick<ObserveRequest, "source" | "dashboard_id">
): RetryAction {
  if (request.source === "observe") return { kind: "create" }
  if (request.source === "prompt" && request.dashboard_id !== null) {
    return { kind: "revise", dashboardId: request.dashboard_id }
  }
  return { kind: "none", dashboardId: request.dashboard_id }
}
