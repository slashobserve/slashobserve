import { describe, expect, it } from "vitest"

import { groupSteps, retryAction, type Step } from "./request-model"

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

describe("groupSteps", () => {
  it("merges consecutive ok steps of one phase and keeps the latest", () => {
    const groups = groupSteps([
      step({ message: "first" }),
      step({ message: "second" }),
      step({ message: "third" }),
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0].count).toBe(3)
    expect(groups[0].step.message).toBe("third")
  })

  it("does not merge across phases", () => {
    const groups = groupSteps([
      step({ phase: "discover" }),
      step({ phase: "query" }),
      step({ phase: "discover" }),
    ])
    expect(groups.map((group) => group.count)).toEqual([1, 1, 1])
  })

  it("does not merge across attempts", () => {
    const groups = groupSteps([
      step({ attempt: 0 }),
      step({ attempt: 0 }),
      step({ attempt: 1 }),
      step({ attempt: 1 }),
    ])
    expect(groups.map((group) => group.count)).toEqual([2, 2])
  })

  it("never merges errors or repairs", () => {
    const groups = groupSteps([
      step({ status: "error" }),
      step({ status: "error" }),
      step({ phase: "repair" }),
      step({ phase: "repair" }),
      step({}),
    ])
    expect(groups.map((group) => group.count)).toEqual([1, 1, 1, 1, 1])
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
