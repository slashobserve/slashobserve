import { describe, expect, test } from "vitest"

import {
  canRepair,
  dashboardCount,
  groupProblems,
  isHealthFilter,
  lastRunProblem,
  pollHealth,
  problemSubject,
  problemText,
  repairAction,
  repairOutcomeText,
  runStartWaitMs,
  sortProblems,
  subjectSummary,
  summaryLabels,
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
    repair: null,
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

const noDataMessage = "query returned no data"

describe("groupProblems", () => {
  test("puts 13 panels without data with one message in one group", () => {
    const groups = groupProblems(
      Array.from({ length: 13 }, (_, i) =>
        problem({
          panel_id: String(i + 1),
          panel_title: `Panel ${String(i + 1).padStart(2, "0")}`,
          message: noDataMessage,
        })
      )
    )
    expect(groups).toHaveLength(1)
    expect(groups[0]?.kind).toBe("no_data")
    expect(groups[0]?.message).toBe(noDataMessage)
    expect(groups[0]?.subjects).toHaveLength(13)
    expect(groups[0]?.subjects[0]).toBe("Panel 01")
  })

  test("keeps two kinds as two groups, failures first", () => {
    const groups = groupProblems([
      problem({ kind: "no_data", panel_title: "Latency" }),
      problem({ kind: "query_failed", panel_title: "Orders", message: "x" }),
    ])
    expect(groups.map((g) => [g.kind, g.subjects])).toEqual([
      ["query_failed", ["Orders"]],
      ["no_data", ["Latency"]],
    ])
  })

  test("keeps one kind with two messages as two groups", () => {
    const groups = groupProblems([
      problem({ kind: "query_failed", panel_title: "A", message: "parse" }),
      problem({ kind: "query_failed", panel_title: "B", message: "timeout" }),
      problem({ kind: "query_failed", panel_title: "C", message: "parse" }),
    ])
    expect(groups.map((g) => [g.message, g.subjects])).toEqual([
      ["parse", ["A", "C"]],
      ["timeout", ["B"]],
    ])
  })

  test("drops empty subjects of dashboard-wide problems", () => {
    expect(groupProblems([problem({ kind: "unopened" })])).toEqual([
      { kind: "unopened", message: null, subjects: [] },
    ])
  })
})

describe("subjectSummary", () => {
  test.each([
    [{ kind: "no_data", message: null, subjects: [] }, null],
    [{ kind: "no_data", message: null, subjects: ["Orders"] }, "Orders"],
    [{ kind: "no_data", message: null, subjects: ["A", "B"] }, "2 panels"],
    [
      {
        kind: "variable_failed",
        message: null,
        subjects: ["Variable env", "Variable pod"],
      },
      "2 variables",
    ],
  ] as const)("names %o as %j", (group, want) => {
    expect(subjectSummary({ ...group, subjects: [...group.subjects] })).toBe(
      want
    )
  })
})

describe("problemText", () => {
  test.each([
    [problem({ message: noDataMessage }), "No data in the last 7 days."],
    [
      problem({ kind: "query_failed", message: "bad_data: parse error" }),
      "Bad_data: parse error.",
    ],
    [problem({ kind: "duplicate_title" }), null],
  ])("reads %o as %j", (p, want) => {
    expect(problemText(p)).toBe(want)
  })
})

describe("repairAction", () => {
  const failing = [problem({ kind: "query_failed", panel_id: "1" })]
  const repair = {
    request_id: "r1",
    version: null,
    error: null,
    updated_at: "2026-11-06T10:00:00Z",
  }

  test.each([
    [
      "an editor with a failing query",
      item({ problems: failing }),
      true,
      {
        kind: "repair",
        disabledReason: null,
      },
    ],
    [
      "an editor with only a duplicate title",
      item({ problems: [problem({ kind: "duplicate_title" })] }),
      true,
      { kind: "open" },
    ],
    [
      "a user who cannot edit",
      item({ can_edit: false, problems: failing }),
      true,
      { kind: "open" },
    ],
    [
      "no agent",
      item({ problems: failing }),
      false,
      { kind: "repair", disabledReason: "No agent is configured" },
    ],
    [
      "a queued repair",
      item({ problems: failing, repair: { ...repair, state: "queued" } }),
      true,
      { kind: "queued" },
    ],
    [
      "a running repair",
      item({ problems: failing, repair: { ...repair, state: "running" } }),
      true,
      { kind: "running" },
    ],
    [
      "a repair that saved a version the check has not read",
      item({
        problems: failing,
        checked_version: 3,
        repair: { ...repair, state: "succeeded", version: 4 },
      }),
      true,
      { kind: "repaired", version: 4 },
    ],
    [
      "a repair the check already covered",
      item({
        problems: failing,
        checked_version: 4,
        repair: { ...repair, state: "succeeded", version: 4 },
      }),
      true,
      { kind: "repair", disabledReason: null },
    ],
    [
      "a covered repair on a dashboard the user cannot edit",
      item({
        can_edit: false,
        problems: failing,
        checked_version: 5,
        repair: { ...repair, state: "succeeded", version: 4 },
      }),
      true,
      { kind: "open" },
    ],
    [
      "a failed repair",
      item({
        problems: failing,
        repair: { ...repair, state: "failed", error: "the model gave up" },
      }),
      false,
      {
        kind: "failed",
        error: "The model gave up.",
        canRetry: true,
        disabledReason: "No agent is configured",
      },
    ],
    [
      "a failed repair on a dashboard the user cannot edit",
      item({
        can_edit: false,
        problems: failing,
        repair: { ...repair, state: "failed" },
      }),
      true,
      { kind: "failed", error: null, canRetry: false, disabledReason: null },
    ],
  ] as const)("%s", (_, it, agentAvailable, want) => {
    expect(repairAction(it, agentAvailable)).toEqual(want)
  })

  test("canRepair needs an editor and a problem the agent fixes", () => {
    expect(canRepair(item({ problems: failing }))).toBe(true)
    expect(canRepair(item({ can_edit: false, problems: failing }))).toBe(false)
    expect(canRepair(item({ problems: [problem({ kind: "unopened" })] }))).toBe(
      false
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

describe("summaryLabels", () => {
  const empty = {
    dashboards: 0,
    failing: 0,
    query_failed: 0,
    variable_failed: 0,
    no_data: 0,
    duplicate_titles: 0,
    unopened: 0,
  }

  test("lists the non-zero counts by severity", () => {
    expect(
      summaryLabels({
        ...empty,
        dashboards: 7,
        failing: 2,
        query_failed: 2,
        no_data: 1,
        duplicate_titles: 1,
        unopened: 3,
      })
    ).toEqual([
      { tone: "critical", label: "2 failing" },
      { tone: "warning", label: "1 without data" },
      { tone: "none", label: "3 not opened in 90 days" },
      { tone: "none", label: "1 duplicate title" },
    ])
  })

  test("drops zero counts", () => {
    expect(summaryLabels(empty)).toEqual([])
    expect(summaryLabels({ ...empty, duplicate_titles: 2 })).toEqual([
      { tone: "none", label: "2 duplicate titles" },
    ])
  })
})

describe("dashboardCount", () => {
  const summary = {
    dashboards: 7,
    failing: 3,
    query_failed: 2,
    variable_failed: 1,
    no_data: 1,
    duplicate_titles: 0,
    unopened: 4,
  }

  test.each([
    [null, "7 dashboards"],
    ["query_failed", "2 dashboards"],
    ["variable_failed", "1 dashboard"],
    ["no_data", "1 dashboard"],
    ["duplicate_title", "0 dashboards"],
    ["unopened", "4 dashboards"],
  ] as const)("counts %j as %j", (kind, want) => {
    expect(dashboardCount(summary, kind)).toBe(want)
  })
})

describe("lastRunProblem", () => {
  test.each([
    [{ ...idle, running: true, last_outcome: "failed" as const }, null],
    [idle, null],
    [
      { ...idle, last_outcome: "failed" as const },
      { tone: "critical", label: "Last run failed" },
    ],
    [
      { ...idle, last_outcome: "interrupted" as const },
      { tone: "warning", label: "Last run was interrupted" },
    ],
    [{ ...idle, last_outcome: null }, null],
  ])("%o reads %j", (job, want) => {
    expect(lastRunProblem(job)).toEqual(want)
  })
})

describe("pollHealth", () => {
  const at = 1_000_000
  const run = { previousStart: idle.last_started_at, at }
  const repair = {
    request_id: "r1",
    version: null,
    error: null,
    updated_at: "2026-11-06T10:00:00Z",
  }

  test.each([
    ["no data yet", undefined, [], null, at, false],
    ["a running job", { ...idle, running: true }, [], null, at, true],
    ["an idle job with no run asked for", idle, [], null, at, false],
    ["a run asked for that has not started", idle, [], run, at + 1000, true],
    [
      "a run asked for that started and finished",
      { ...idle, last_started_at: "2026-11-06T10:00:00Z" },
      [],
      run,
      at + 1000,
      false,
    ],
    [
      "a run that never started in time",
      idle,
      [],
      run,
      at + runStartWaitMs,
      false,
    ],
    [
      "a queued repair",
      idle,
      [item({ repair: { ...repair, state: "queued" } })],
      null,
      at,
      true,
    ],
    [
      "a running repair",
      idle,
      [item({ repair: { ...repair, state: "running" } })],
      null,
      at,
      true,
    ],
    [
      "only finished repairs",
      idle,
      [
        item({ repair: { ...repair, state: "succeeded", version: 2 } }),
        item({ repair: { ...repair, state: "failed" } }),
      ],
      null,
      at,
      false,
    ],
  ])("%s", (_, job, items, request, now, want) => {
    expect(pollHealth(job, items, request, now)).toBe(want)
  })
})

describe("isHealthFilter", () => {
  test.each([
    ["no_data", true],
    ["unopened", true],
    ["imported", true],
    ["placeholder", false],
    [undefined, false],
  ])("%j is %s", (value, want) => {
    expect(isHealthFilter(value)).toBe(want)
  })
})
