import { describe, expect, it } from "vitest"

import type { PanelDataResponse } from "@/dashboards/queries"

import { summaryText } from "./results-section"

function dataOf(frames: number, rows: number, step: number, took: number) {
  return {
    frames: Array.from({ length: frames }, () => ({})),
    meta: { rows, step_ms: step, elapsed_ms: took },
  } as unknown as PanelDataResponse
}

describe("summaryText", () => {
  it("counts series for a graph", () => {
    expect(summaryText(dataOf(3, 0, 15000, 42), "graph")).toContain("3 series")
  })

  it("counts rows for a table and uses the singular", () => {
    expect(summaryText(dataOf(0, 1, 0, 5), "table")).toMatch(/^1 row/)
    expect(summaryText(dataOf(0, 1200, 0, 5), "table")).toMatch(/^1,200 rows/)
  })
})
