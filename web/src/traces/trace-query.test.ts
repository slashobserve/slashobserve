import { describe, expect, it } from "vitest"

import type { PanelDataResponse } from "@/dashboards/queries"

import {
  criticalPathDisabledText,
  traceNotes,
  traceNotice,
  traceSizeText,
} from "./trace-query"

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

const cut = { truncated: true, rows: 20000, rows_total: 23412 }

describe("traceNotes", () => {
  it("says nothing for a whole trace, cut or not", () => {
    expect(traceNotes(meta({}))).toEqual([])
    expect(traceNotes(meta(cut))).toEqual([])
  })

  it("says when Tempo did not search every block", () => {
    expect(traceNotes(meta({ partial: true }))).toEqual([
      "Tempo did not search every block; some spans may be missing.",
    ])
  })
})

describe("traceSizeText", () => {
  it("rounds Tempo's payload size and is absent without it", () => {
    expect(traceSizeText(meta({ bytes: 9_300_000 }))).toBe("about 9 MB")
    expect(traceSizeText(meta({ bytes: 512_000 }))).toBe("about 512 kB")
    expect(traceSizeText(meta({}))).toBeNull()
    expect(traceSizeText(meta({ bytes: 0 }))).toBeNull()
  })
})

describe("traceNotice", () => {
  it("explains folding when groups exist, with the size when known", () => {
    expect(traceNotice(meta({}), 12400, true)).toBe(
      "This trace has 12,400 spans. Repeated spans are folded into their parent so the page stays quick. Nothing is lost: every folded group can be opened."
    )
    expect(traceNotice(meta({ bytes: 9_000_000 }), 12400, true)).toBe(
      "This trace has 12,400 spans (about 9 MB). Repeated spans are folded into their parent so the page stays quick. Nothing is lost: every folded group can be opened."
    )
  })

  it("says what is loaded when the trace is cut, whether or not it folds", () => {
    const text =
      "This trace has 23,412 spans. The first 20,000 by start time are loaded; the rest are not."
    expect(traceNotice(meta(cut), 20000, true)).toBe(text)
    expect(traceNotice(meta(cut), 20000, false)).toBe(text)
    expect(traceNotice(meta({ ...cut, bytes: 9_000_000 }), 20000, false)).toBe(
      "This trace has 23,412 spans. The first 20,000 (about 9 MB) by start time are loaded; the rest are not."
    )
  })

  it("is absent for a whole trace without groups", () => {
    expect(traceNotice(meta({}), 300, false)).toBeNull()
  })
})

describe("criticalPathDisabledText", () => {
  it("disables the critical path only for a cut trace", () => {
    expect(criticalPathDisabledText(meta(cut))).toBe(
      "Critical path needs all spans. This trace is cut at 20,000."
    )
    expect(criticalPathDisabledText(meta({}))).toBeUndefined()
  })
})
