import { describe, expect, it } from "vitest"

import type { TraceRow } from "./trace-rows"
import {
  histogramPercentile,
  p95Label,
  noTracesText,
  partialNote,
  partialReason,
} from "./trace-stats"

const histogram = [
  { le_ms: 64, count: 10 },
  { le_ms: 128, count: 80 },
  { le_ms: 256, count: 10 },
]

describe("histogramPercentile", () => {
  it.each([
    [95, 256],
    [50, 128],
    [10, 64],
    [100, 256],
  ])("answers the bucket bound that holds the p%i", (p, bound) => {
    expect(histogramPercentile(histogram, p)).toBe(bound)
  })

  it("has no percentile for an empty histogram", () => {
    expect(histogramPercentile([], 95)).toBeNull()
    expect(histogramPercentile([{ le_ms: 64, count: 0 }], 95)).toBeNull()
  })
})

describe("p95Label", () => {
  it("names every trace without a service filter", () => {
    expect(p95Label({})).toBe("p95 of all traces")
  })

  it("names the filtered service", () => {
    expect(p95Label({ service: "checkout" })).toBe("p95 of all checkout traces")
  })
})

function row(traceId: string, start: number | null): TraceRow {
  return {
    traceId,
    start,
    service: "checkout",
    span: "POST /checkout",
    durationMs: 10,
    spans: null,
    errors: null,
  }
}

describe("partialNote", () => {
  it("covers the span of the rows' start times", () => {
    const rows = [
      row("a", Date.UTC(2026, 10, 6, 14, 12)),
      row("b", Date.UTC(2026, 10, 6, 14, 6)),
      row("c", Date.UTC(2026, 10, 6, 14, 20)),
    ]
    expect(partialNote(rows, "UTC")).toEqual({
      text: "The 3 found cover 2026-11-06 14:06:00 to 2026-11-06 14:20:00.",
      covers: "2026-11-06 14:06:00 to 2026-11-06 14:20:00",
      from: "2026-11-06T14:06:00.000Z",
      to: "2026-11-06T14:20:00.000Z",
    })
  })

  it("skips rows without a start and says nothing when none has one", () => {
    const at = Date.UTC(2026, 10, 6, 14, 6)
    expect(partialNote([row("a", null), row("b", at)], "UTC")?.text).toBe(
      "The 2 found cover 2026-11-06 14:06:00 to 2026-11-06 14:06:00."
    )
    expect(partialNote([row("a", null)], "UTC")).toBeNull()
    expect(partialNote([], "UTC")).toBeNull()
  })
})

describe("partialReason", () => {
  const base = {
    order: "newest first",
    pageSize: 50,
    hasNext: true,
    cut: false,
  }

  it("says what order a full page is in and where the rest is", () => {
    expect(partialReason({ ...base, rowCount: 50 })).toBe(
      "This page shows 50 traces, newest first; more matches are on the next pages."
    )
    expect(
      partialReason({ ...base, rowCount: 50, order: "slowest first" })
    ).toBe(
      "This page shows 50 traces, slowest first; more matches are on the next pages."
    )
  })

  it("says the same for a full page that Tempo cut short", () => {
    expect(partialReason({ ...base, rowCount: 50, cut: true })).toBe(
      "This page shows 50 traces, newest first; more matches are on the next pages."
    )
  })

  it("counts a single trace in the singular", () => {
    expect(partialReason({ ...base, rowCount: 1 })).toBe(
      "This page shows 1 trace, newest first; more matches are on the next pages."
    )
  })

  it("points back to the earlier pages on the last page", () => {
    expect(partialReason({ ...base, rowCount: 20, hasNext: false })).toBe(
      "This page shows 20 traces, newest first; the rest are on the earlier pages."
    )
  })

  it("blames the time limit when the page is not full", () => {
    expect(
      partialReason({ ...base, rowCount: 12, hasNext: false, cut: true })
    ).toBe(
      "Tempo stopped searching at its time limit, so some matching traces may be missing."
    )
  })

  it("points to the next page when the time limit cut a page short", () => {
    expect(partialReason({ ...base, rowCount: 12, cut: true })).toBe(
      "Tempo stopped searching at its time limit, so these are the newest matches it found; the next page searches further back."
    )
  })
})

describe("noTracesText", () => {
  it("says nothing matched in a complete search", () => {
    expect(noTracesText(false, "the last 24 hours")).toBe(
      "No traces match in the last 24 hours."
    )
  })

  it("does not claim nothing matched when the search was cut", () => {
    expect(noTracesText(true, "the last 24 hours")).toBe(
      "Tempo stopped searching at its time limit before it found a trace in the last 24 hours. Choose a shorter range or add a filter."
    )
  })
})
