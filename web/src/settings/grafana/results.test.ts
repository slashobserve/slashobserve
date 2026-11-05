import { describe, expect, it } from "vitest"

import { groupResults, summaryText, type GrafanaImportResult } from "./results"

function result(
  uid: string,
  outcome: GrafanaImportResult["outcome"],
  unverified = 0,
  placeholders = 0
): GrafanaImportResult {
  return {
    uid,
    title: uid,
    outcome,
    dashboard_id: outcome === "failed" ? null : `id-${uid}`,
    unverified_panels: unverified,
    placeholder_panels: placeholders,
    error: outcome === "failed" ? "broken" : null,
  }
}

describe("groupResults", () => {
  it("groups each result by outcome and repair need, keeping the server's order", () => {
    const results = [
      result("a", "imported"),
      result("b", "failed"),
      result("c", "updated", 2),
      result("d", "skipped"),
      result("e", "imported", 0, 1),
      result("f", "updated"),
      result("g", "failed"),
    ]
    const groups = groupResults(results)
    expect(groups.failed.map((r) => r.uid)).toEqual(["b", "g"])
    expect(groups.needsRepair.map((r) => r.uid)).toEqual(["c", "e"])
    expect(groups.imported.map((r) => r.uid)).toEqual(["a", "f"])
    expect(groups.skipped.map((r) => r.uid)).toEqual(["d"])
  })

  it("returns empty groups for no results", () => {
    expect(groupResults([])).toEqual({
      failed: [],
      needsRepair: [],
      imported: [],
      skipped: [],
    })
  })
})

describe("summaryText", () => {
  it.each([
    [{ done: 10, failed: 2, skipped: 3 }, "5 imported, 2 failed, 3 skipped"],
    [{ done: 0, failed: 0, skipped: 0 }, "0 imported, 0 failed, 0 skipped"],
  ])("counts %o", (summary, want) => {
    expect(summaryText(summary)).toBe(want)
  })
})
