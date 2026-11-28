import { describe, expect, it } from "vitest"

import {
  isSavedAnswerStep,
  stepText,
  timelinePhases,
  type Step,
} from "./request-model"
import { must } from "@/test/must"

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
    const run = must(
      timelinePhases([
        step({ phase: "discover", tool: "list_metrics" }),
        step({ phase: "discover", tool: "get_schema" }),
        step({ phase: "discover", tool: "list_metrics" }),
      ])[0]
    )
    expect(run.tools).toEqual([
      { tool: "list_metrics", count: 2 },
      { tool: "get_schema", count: 1 },
    ])
  })

  it("counts errors and sums durations", () => {
    const run = must(
      timelinePhases([
        step({ status: "error", duration_ms: 120 }),
        step({ status: "ok", duration_ms: 30 }),
        step({ status: "error", duration_ms: 50 }),
      ])[0]
    )
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

describe("stepText", () => {
  const base: Step = {
    at: "2026-01-01T00:00:00Z",
    attempt: 0,
    phase: "save",
    tool: "save_finding",
    status: "ok",
    message: "Saved the answer",
    duration_ms: 1,
  }

  it("rewrites the stored id of a finding saved before the step read as a sentence", () => {
    expect(
      stepText({ ...base, message: "Saved finding 99a52b2f-9aca-4f4e" })
    ).toBe("Saved the answer")
    expect(stepText({ ...base, message: "Saved panel 1 of 2: Up" })).toBe(
      "Saved panel 1 of 2: Up"
    )
  })

  it("finds the step that saved the answer", () => {
    expect(isSavedAnswerStep(base)).toBe(true)
    expect(isSavedAnswerStep({ ...base, status: "error" })).toBe(false)
    expect(isSavedAnswerStep({ ...base, tool: "update_panels" })).toBe(false)
    expect(
      isSavedAnswerStep({
        ...base,
        message: "Reused the result of an identical call",
      })
    ).toBe(false)
    expect(
      isSavedAnswerStep({ ...base, message: "Saved finding 99a52b2f" })
    ).toBe(true)
  })
})
