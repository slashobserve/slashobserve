import { describe, expect, test } from "vitest"

import {
  canRepair,
  isHealthKind,
  jobLine,
  pollHealth,
  problemSubject,
  repairOutcomeText,
  repairReason,
  runStartWaitMs,
  sortProblems,
  type HealthItem,
  type HealthJob,
  type HealthProblem,
} from "./health-labels"

function problem(fields: Partial<HealthProblem>): HealthProblem {
  return {
    kind: "no_data",
    panel_id: null,
    panel_title: null,
    variable: null,
    code: null,
    message: null,
    ...fields,
  }
}

function item(fields: Partial<HealthItem>): HealthItem {
  return {
    id: "d1",
    title: "Checkout",
    url: "/d/d1/checkout",
    folder: { id: "f1", title: "General" },
    can_edit: true,
    checked_version: 3,
    checked_at: "2026-11-05T10:00:00Z",
    last_viewed_at: null,
    skipped: null,
    problems: [],
    ...fields,
  } as HealthItem
}

const idle: HealthJob = {
  last_started_at: "2026-11-06T09:00:00Z",
  last_finished_at: "2026-11-06T09:02:00Z",
  last_outcome: "succeeded",
  running: false,
}

describe("sortProblems", () => {
  test("puts failures first, then no data, then dashboard-wide problems, each by subject", () => {
    const sorted = sortProblems([
      problem({ kind: "unopened" }),
      problem({ kind: "no_data", panel_id: "2", panel_title: "Latency" }),
      problem({ kind: "duplicate_title" }),
      problem({ kind: "no_data", panel_id: "1", panel_title: "Errors" }),
      problem({ kind: "variable_failed", variable: "env" }),
      problem({ kind: "query_failed", panel_id: "3", panel_title: "Orders" }),
    ])
    expect(sorted.map((p) => `${p.kind} ${problemSubject(p)}`)).toEqual([
      "query_failed Orders",
      "variable_failed Variable env",
      "no_data Errors",
      "no_data Latency",
      "duplicate_title ",
      "unopened ",
    ])
  })
})

describe("problemSubject", () => {
  test.each([
    [problem({ variable: "env", panel_title: "x" }), "Variable env"],
    [problem({ panel_id: "7", panel_title: "Orders" }), "Orders"],
    [problem({ panel_id: "7" }), "Panel 7"],
    [problem({ kind: "unopened" }), ""],
  ])("names %o as %j", (p, want) => {
    expect(problemSubject(p)).toBe(want)
  })
})

describe("repair", () => {
  const failing = [problem({ kind: "query_failed", panel_id: "1" })]

  test.each([
    ["an editor with a failing query", item({ problems: failing }), true, null],
    [
      "an editor with only a duplicate title",
      item({ problems: [problem({ kind: "duplicate_title" })] }),
      false,
      "Nothing here the agent can repair",
    ],
    [
      "a viewer",
      item({ can_edit: false, problems: failing }),
      false,
      "You cannot edit this dashboard",
    ],
  ])("%s", (_, it, repairable, reason) => {
    expect(canRepair(it)).toBe(repairable)
    expect(repairReason(it, true)).toBe(reason)
  })

  test("no agent blocks a repairable dashboard", () => {
    expect(repairReason(item({ problems: failing }), false)).toBe(
      "No agent is configured"
    )
  })

  test("the outcome names a queued repair or why it was skipped", () => {
    expect(
      repairOutcomeText({
        queued: [{ dashboard_id: "d1", revision_id: "r1" }],
        skipped: [],
      } as never)
    ).toMatch(/^Repair queued\./)
    expect(
      repairOutcomeText({
        queued: [],
        skipped: [{ dashboard_id: "d1", reason: "active" }],
      } as never)
    ).toBe(
      "No repair queued. A revision is already queued or running on this dashboard."
    )
  })
})

describe("jobLine", () => {
  test.each([
    [{ ...idle, running: true }, "running", "Checking dashboards now", null],
    [idle, "ok", "Last run finished", idle.last_finished_at],
    [
      { ...idle, last_outcome: "failed" as const },
      "critical",
      "Last run failed",
      idle.last_finished_at,
    ],
    [
      { ...idle, last_outcome: "interrupted" as const, last_finished_at: null },
      "warning",
      "Last run was interrupted",
      idle.last_started_at,
    ],
    [
      {
        last_started_at: null,
        last_finished_at: null,
        last_outcome: null,
        running: false,
      },
      "none",
      "No run has finished yet",
      null,
    ],
  ])("%o reads %s %j", (job, tone, label, at) => {
    expect(jobLine(job)).toEqual({ tone, label, at })
  })
})

describe("pollHealth", () => {
  const at = 1_000_000
  const run = { previousStart: idle.last_started_at, at }

  test.each([
    ["no data yet", undefined, null, at, false],
    ["a running job", { ...idle, running: true }, null, at, true],
    ["an idle job with no run asked for", idle, null, at, false],
    ["a run asked for that has not started", idle, run, at + 1000, true],
    [
      "a run asked for that started and finished",
      { ...idle, last_started_at: "2026-11-06T10:00:00Z" },
      run,
      at + 1000,
      false,
    ],
    ["a run that never started in time", idle, run, at + runStartWaitMs, false],
  ])("%s", (_, job, request, now, want) => {
    expect(pollHealth(job, request, now)).toBe(want)
  })
})

describe("isHealthKind", () => {
  test.each([
    ["no_data", true],
    ["unopened", true],
    ["placeholder", false],
    [undefined, false],
  ])("%j is %s", (value, want) => {
    expect(isHealthKind(value)).toBe(want)
  })
})
