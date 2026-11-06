import { describe, expect, it } from "vitest"

import {
  durationScale,
  durationTicks,
  durationTickText,
  scatterPoints,
  timeTicks,
  timeTickText,
} from "./scatter"
import type { TraceRow } from "./trace-rows"

function row(overrides: Partial<TraceRow> & { traceId: string }): TraceRow {
  return {
    start: null,
    service: "",
    span: "",
    durationMs: null,
    spans: null,
    errors: null,
    ...overrides,
  }
}

describe("durationScale", () => {
  it("puts 1 ms at the bottom and the decade above the slowest trace at the top", () => {
    const scale = durationScale(2300)
    expect(scale.minMs).toBe(1)
    expect(scale.maxMs).toBe(10_000)
    expect(scale.y(1)).toBe(0)
    expect(scale.y(10_000)).toBe(1)
    expect(scale.y(100)).toBeCloseTo(0.5)
  })

  it("keeps a trace that is exactly a decade below the top edge", () => {
    expect(durationScale(1000).maxMs).toBe(10_000)
  })

  it("clamps durations below a millisecond and above the top", () => {
    const scale = durationScale(5)
    expect(scale.maxMs).toBe(10)
    expect(scale.y(0.2)).toBe(0)
    expect(scale.y(50)).toBe(1)
  })
})

describe("durationTicks", () => {
  it("lists the decades inside the scale", () => {
    expect(durationTicks(2300)).toEqual([10, 100, 1000, 10_000])
    expect(durationTicks(5)).toEqual([10])
  })

  it("keeps at most four ticks, always including the top", () => {
    expect(durationTicks(500_000)).toEqual([100, 10_000, 1_000_000])
  })
})

describe("durationTickText", () => {
  it("reads in milliseconds below a second and in seconds above", () => {
    expect(durationTickText(10)).toBe("10 ms")
    expect(durationTickText(1000)).toBe("1 s")
    expect(durationTickText(10_000)).toBe("10 s")
  })
})

describe("scatterPoints", () => {
  const range = { start: 1000, end: 3000 }

  it("maps start across the range and duration through the log scale", () => {
    const points = scatterPoints(
      [
        row({ traceId: "a", start: 2000, durationMs: 100, errors: 0 }),
        row({ traceId: "b", start: 3000, durationMs: 10_000, errors: 2 }),
      ],
      range,
      2300
    )
    expect(points).toEqual([
      { traceId: "a", x: 0.5, y: 0.5, error: false },
      { traceId: "b", x: 1, y: 1, error: true },
    ])
  })

  it("drops rows without a start or a duration", () => {
    const points = scatterPoints(
      [
        row({ traceId: "a", start: null, durationMs: 100 }),
        row({ traceId: "b", start: 2000, durationMs: null }),
        row({ traceId: "c", start: 2000, durationMs: 100 }),
      ],
      range,
      100
    )
    expect(points.map((point) => point.traceId)).toEqual(["c"])
  })

  it("keeps a trace outside the range on the edge", () => {
    const [point] = scatterPoints(
      [row({ traceId: "a", start: 500, durationMs: 1 })],
      range,
      1
    )
    expect(point?.x).toBe(0)
  })
})

describe("timeTicks", () => {
  it("picks the finest step that fits the tick budget, aligned to the step", () => {
    const start = Date.UTC(2026, 10, 7, 13, 17)
    const end = Date.UTC(2026, 10, 7, 14, 17)
    const ticks = timeTicks({ start, end }, 6)
    expect(ticks).toEqual([
      Date.UTC(2026, 10, 7, 13, 20),
      Date.UTC(2026, 10, 7, 13, 30),
      Date.UTC(2026, 10, 7, 13, 40),
      Date.UTC(2026, 10, 7, 13, 50),
      Date.UTC(2026, 10, 7, 14, 0),
      Date.UTC(2026, 10, 7, 14, 10),
    ])
  })

  it("is empty for an empty range", () => {
    expect(timeTicks({ start: 5, end: 5 }, 6)).toEqual([])
  })
})

describe("timeTickText", () => {
  it("shows the clock time inside a day and the date beyond it", () => {
    const at = Date.UTC(2026, 10, 7, 13, 20)
    const hour = { start: at, end: at + 3_600_000 }
    const week = { start: at, end: at + 7 * 86_400_000 }
    expect(timeTickText(at, hour)).toMatch(/^\d\d:\d\d$/)
    expect(timeTickText(at, week)).toMatch(/^\d\d-\d\d \d\d:\d\d$/)
  })
})
