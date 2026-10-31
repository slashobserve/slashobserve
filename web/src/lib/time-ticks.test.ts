import { describe, expect, it } from "vitest"

import { timeLabel, timeTicks } from "./time-ticks"

const minute = 60_000

function steps(count: number, stepMs: number, start: number): number[] {
  return Array.from({ length: count }, (_, i) => start + i * stepMs)
}

describe("timeTicks", () => {
  it("has no ticks without columns or room", () => {
    expect(timeTicks([], 5, undefined)).toEqual([])
    expect(timeTicks([0, 1], 0, undefined)).toEqual([])
  })

  it("puts ticks where the clock crosses a round interval", () => {
    const start = new Date(2026, 0, 5, 10, 1, 0).getTime()
    const xs = steps(60, minute, start)
    const ticks = timeTicks(xs, 6, undefined)
    expect(ticks.length).toBeGreaterThan(1)
    expect(ticks.length).toBeLessThanOrEqual(7)
    for (const tick of ticks) {
      expect(new Date(xs[tick.index]).getMinutes() % 10).toBe(0)
    }
  })

  it("never exceeds the tick budget by more than the edge tick", () => {
    const start = new Date(2026, 0, 5, 0, 0, 0).getTime()
    const xs = steps(361, 10_000, start)
    expect(timeTicks(xs, 4, undefined).length).toBeLessThanOrEqual(5)
  })

  it("places hour ticks on the whole hour of the zone", () => {
    const xs = steps(181, minute, Date.UTC(2026, 0, 5, 0, 0, 0))
    const labels = (zone: string) =>
      timeTicks(xs, 4, zone).map((tick) => tick.label)
    expect(labels("UTC")).toEqual(["00:00", "01:00", "02:00", "03:00"])
    expect(labels("Asia/Kolkata")).toEqual(["06:00", "07:00", "08:00"])
  })
})

describe("timeLabel", () => {
  const ms = new Date(2026, 0, 5, 14, 7, 9).getTime()

  it("shows seconds under five minutes, then hours and minutes", () => {
    expect(timeLabel(ms, 2 * minute, undefined)).toBe("14:07:09")
    expect(timeLabel(ms, 60 * minute, undefined)).toBe("14:07")
  })

  it("adds the date for longer spans", () => {
    expect(timeLabel(ms, 2 * 24 * 60 * minute, undefined)).toBe("01-05 14:07")
    expect(timeLabel(ms, 10 * 24 * 60 * minute, undefined)).toBe("01-05")
  })

  it("reads the clock of the given zone", () => {
    const at = Date.UTC(2026, 0, 5, 20, 30, 9)
    expect(timeLabel(at, 2 * minute, "UTC")).toBe("20:30:09")
    expect(timeLabel(at, 60 * minute, "Asia/Kolkata")).toBe("02:00")
    expect(timeLabel(at, 2 * 24 * 60 * minute, "Asia/Kolkata")).toBe(
      "01-06 02:00"
    )
  })
})
