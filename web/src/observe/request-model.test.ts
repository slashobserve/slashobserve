import { describe, expect, it } from "vitest"

import { retryAction, timelinePhases, type Step } from "./request-model"

function step(overrides: Partial<Step>): Step {
  return {
    at: "2026-01-01T00:00:00Z",
    attempt: 0,
    phase: "query",
    tool: "query_range",
    status: "ok",
    message: "Ran a query",
    duration_ms: 1,
    ...overrides,
  }
}

describe("timelinePhases", () => {
  it("starts a new run at each phase change", () => {
    const runs = timelinePhases([
      step({ phase: "discover" }),
      step({ phase: "discover" }),
      step({ phase: "query" }),
      step({ phase: "discover" }),
    ])
    expect(runs.map((run) => [run.phase, run.steps.length])).toEqual([
      ["discover", 2],
      ["query", 1],
      ["discover", 1],
    ])
  })

  it("starts a new run at an attempt boundary", () => {
    const runs = timelinePhases([
      step({ attempt: 0 }),
      step({ attempt: 0 }),
      step({ attempt: 1 }),
    ])
    expect(runs.map((run) => run.steps.length)).toEqual([2, 1])
  })

  it("keys each run by its attempt and first step index", () => {
    const runs = timelinePhases([
      step({ phase: "discover" }),
      step({ phase: "query", attempt: 0 }),
      step({ phase: "query", attempt: 1 }),
    ])
    expect(runs.map((run) => run.key)).toEqual(["0:0", "0:1", "1:2"])
  })

  it("counts tools in first-seen order", () => {
    const [run] = timelinePhases([
      step({ phase: "discover", tool: "list_metrics" }),
      step({ phase: "discover", tool: "get_schema" }),
      step({ phase: "discover", tool: "list_metrics" }),
    ])
    expect(run.tools).toEqual([
      { tool: "list_metrics", count: 2 },
      { tool: "get_schema", count: 1 },
    ])
  })

  it("counts errors and sums durations", () => {
    const [run] = timelinePhases([
      step({ status: "error", duration_ms: 120 }),
      step({ status: "ok", duration_ms: 30 }),
      step({ status: "error", duration_ms: 50 }),
    ])
    expect(run.errors).toBe(2)
    expect(run.durationMs).toBe(200)
  })

  it("gives every repair step its own run", () => {
    const runs = timelinePhases([
      step({ phase: "repair" }),
      step({ phase: "repair" }),
      step({ phase: "query" }),
    ])
    expect(runs.map((run) => [run.phase, run.steps.length])).toEqual([
      ["repair", 1],
      ["repair", 1],
      ["query", 1],
    ])
  })
})

describe("retryAction", () => {
  const id = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"

  it("builds a new dashboard again for observe requests", () => {
    expect(retryAction({ source: "observe", dashboard_id: null })).toEqual({
      kind: "create",
    })
    expect(retryAction({ source: "observe", dashboard_id: id })).toEqual({
      kind: "create",
    })
  })

  it("revises the same dashboard again for prompt follow-ups", () => {
    expect(retryAction({ source: "prompt", dashboard_id: id })).toEqual({
      kind: "revise",
      dashboardId: id,
    })
  })

  it("offers no retry for a prompt request without a dashboard", () => {
    expect(retryAction({ source: "prompt", dashboard_id: null })).toEqual({
      kind: "none",
      dashboardId: null,
    })
  })

  it("offers no retry for comment revisions", () => {
    expect(retryAction({ source: "comment", dashboard_id: id })).toEqual({
      kind: "none",
      dashboardId: id,
    })
  })
})
