import { describe, expect, it } from "vitest"

import type { PanelDataResponse } from "@/dashboards/queries"

import { traceNotes } from "./trace-query"

function meta(
  change: Partial<PanelDataResponse["meta"]>
): PanelDataResponse["meta"] {
  return {
    kind: "instant",
    from: "now-24h",
    to: "now",
    step_ms: 0,
    queries: [],
    cached: false,
    executed_at: "2026-11-07T00:00:00Z",
    truncated: false,
    partial: false,
    rows: 12,
    ...change,
  }
}

describe("traceNotes", () => {
  it("says nothing for a whole trace", () => {
    expect(traceNotes(meta({}))).toEqual([])
  })

  it("says how many spans were cut by start time", () => {
    expect(
      traceNotes(meta({ truncated: true, rows: 20000, rows_total: 23412 }))
    ).toEqual([
      "Showing the first 20,000 of 23,412 spans by start time. The rest are not loaded.",
    ])
  })

  it("says when Tempo did not search every block", () => {
    expect(traceNotes(meta({ partial: true }))).toEqual([
      "Tempo did not search every block; some spans may be missing.",
    ])
  })

  it("keeps both notes in order", () => {
    expect(
      traceNotes(
        meta({ truncated: true, rows: 1, rows_total: 2, partial: true })
      )
    ).toHaveLength(2)
  })
})
