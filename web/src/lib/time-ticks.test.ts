import { describe, expect, it } from "vitest"

import { timeLabel, timeTicks } from "./time-ticks"

const minute = 60_000

function steps(count: number, stepMs: number, start: number): number[] {
  return Array.from({ length: count }, (_, i) => start + i * stepMs)
}

describe("timeTicks", () => {
  it("has no ticks without columns or room", () => {
    expect(timeTicks([], 5)).toEqual([])
    expect(timeTicks([0, 1], 0)).toEqual([])
  })

  it("puts ticks where the clock crosses a round interval", () => {
    const start = new Date(2026, 0, 5, 10, 1, 0).getTime()
    const xs = steps(60, minute, start)
    const ticks = timeTicks(xs, 6)
    expect(ticks.length).toBeGreaterThan(1)
    expect(ticks.length).toBeLessThanOrEqual(7)
    for (const tick of ticks) {
      expect(new Date(xs[tick.index]).getMinutes() % 10).toBe(0)
    }
  })

  it("never exceeds the tick budget by more than the edge tick", () => {
    const start = new Date(2026, 0, 5, 0, 0, 0).getTime()
    const xs = steps(361, 10_000, start)
    expect(timeTicks(xs, 4).length).toBeLessThanOrEqual(5)
  })
})

describe("timeLabel", () => {
  const ms = new Date(2026, 0, 5, 14, 7, 9).getTime()

  it("shows seconds under five minutes, then hours and minutes", () => {
    expect(timeLabel(ms, 2 * minute)).toBe("14:07:09")
    expect(timeLabel(ms, 60 * minute)).toBe("14:07")
  })

  it("adds the date for longer spans", () => {
    expect(timeLabel(ms, 2 * 24 * 60 * minute)).toBe("01-05 14:07")
    expect(timeLabel(ms, 10 * 24 * 60 * minute)).toBe("01-05")
  })
})
