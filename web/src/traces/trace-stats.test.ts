import { describe, expect, it } from "vitest"

import type { TraceRow } from "./trace-rows"
import { histogramPercentile, p95Label, partialNote } from "./trace-stats"

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
