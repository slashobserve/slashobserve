import { describe, expect, test } from "vitest"

import { presenceLabel, scopeLine, statCells } from "./stats"

const stats = {
  runs: 14,
  failed: 2,
  cost_usd: 1.234,
  median_queue_wait_seconds: 12,
}

describe("statCells", () => {
  test("lists runs, failed, cost and median wait", () => {
    expect(statCells(stats)).toEqual([
      { label: "Runs", value: "14" },
      { label: "Failed", value: "2" },
      { label: "Cost", value: "$1.23" },
      { label: "Median wait", value: "12 s" },
    ])
  })

  test("is empty when the caller may not see the stats", () => {
    expect(statCells(null)).toEqual([])
  })

  test.each([
    [null, "Not priced"],
    [0, "$0.00"],
    [0.004, "<$0.01"],
    [0.01, "$0.01"],
    [1234.5, "$1,234.50"],
  ])("cost %s reads %s", (cost, text) => {
    expect(statCells({ ...stats, cost_usd: cost })[2]?.value).toBe(text)
  })

  test.each([
    [null, "No runs"],
    [0, "0 s"],
    [12, "12 s"],
    [59.4, "59 s"],
    [59.6, "1 min"],
    [180, "3 min"],
    [3600, "1 h"],
    [7200, "2 h"],
  ])("median wait %s reads %s", (seconds, text) => {
    expect(
      statCells({ ...stats, median_queue_wait_seconds: seconds })[3]?.value
    ).toBe(text)
  })
})

describe("scopeLine", () => {
  test("names whose runs are counted", () => {
    expect(scopeLine("workspace")).toBe("Last 7 days, everyone")
    expect(scopeLine("you")).toBe("Last 7 days, yours")
  })
})

describe("presenceLabel", () => {
  const now = Date.parse("2026-11-11T10:00:00Z")

  test.each([
    ["working", "Working"],
    ["listening", "Listening"],
  ] as const)("says %s as %s whatever the last seen time", (presence, text) => {
    expect(presenceLabel(presence, "2026-11-11T09:00:00Z", now)).toBe(text)
  })

  test("gives the time since an offline agent was last seen", () => {
    expect(presenceLabel("offline", "2026-11-11T09:55:00Z", now)).toBe(
      "Offline, last seen 5 min ago"
    )
  })

  test("says Never seen when it never was", () => {
    expect(presenceLabel("offline", null, now)).toBe("Never seen")
  })
})
