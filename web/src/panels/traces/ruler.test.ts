import { describe, expect, it } from "vitest"

import { niceTicks, tickAnchor, tickLimit } from "./ruler"

function nth<T>(items: ArrayLike<T>, index: number): T {
  const item = items[index]
  expect(item).toBeDefined()
  return item as T
}

describe("niceTicks", () => {
  it("steps a round duration in quarters", () => {
    expect(niceTicks(1000)).toEqual([0, 250, 500, 750, 1000])
  })

  it("ends at or before the end of an uneven duration", () => {
    expect(niceTicks(1249.6)).toEqual([0, 250, 500, 750, 1000])
  })

  it("steps a short duration by a round number", () => {
    expect(niceTicks(7)).toEqual([0, 2, 4, 6])
  })

  it("gives only the start for an empty duration", () => {
    expect(niceTicks(0)).toEqual([0])
  })

  it("keeps between four and six round ticks across magnitudes", () => {
    for (const duration of [0.37, 3, 12, 99, 312.4, 4321, 61_000, 9e6]) {
      const ticks = niceTicks(duration)
      expect(ticks.length).toBeGreaterThanOrEqual(4)
      expect(ticks.length).toBeLessThanOrEqual(6)
      expect(ticks.at(-1)!).toBeLessThanOrEqual(duration)
      const step = nth(ticks, 1) / 10 ** Math.floor(Math.log10(nth(ticks, 1)))
      expect([1, 2, 2.5, 5]).toContainEqual(Number(step.toFixed(6)))
    }
  })
})

describe("niceTicks in a zoomed window", () => {
  it("puts the ticks on round offsets from the start of the trace", () => {
    const origin = 65.55
    const ticks = niceTicks(300, 6, origin)
    const labels = ticks.map((tick) => origin + tick)
    expect(labels).toEqual([100, 150, 200, 250, 300, 350])
    expect(ticks[0]).toBeCloseTo(34.45)
  })

  it("matches the unzoomed ticks at the start of the trace", () => {
    expect(niceTicks(1000, 6, 0)).toEqual(niceTicks(1000))
  })

  it("keeps four to six round ticks in the window", () => {
    for (const origin of [3.3, 65.55, 480, 1234.5]) {
      const ticks = niceTicks(300, 6, origin)
      expect(ticks.length).toBeGreaterThanOrEqual(4)
      expect(ticks.length).toBeLessThanOrEqual(6)
      for (const tick of ticks) {
        expect(tick).toBeGreaterThanOrEqual(0)
        expect(tick).toBeLessThanOrEqual(300)
      }
    }
  })
})

describe("niceTicks in a window no step fills", () => {
  const origin = 6.05
  const window = 7.58

  it("still gives round ticks to read when no step makes four to six", () => {
    const ticks = niceTicks(window, 6, origin)
    const labels = ticks.map((tick) => origin + tick)
    expect(labels).toEqual([7.5, 10, 12.5])
  })

  it("gives round ticks inside the window whatever its start", () => {
    for (const start of [0.4, 6.05, 13.6, 21.9, 480.3]) {
      for (const duration of [2.2, 7.58, 33.3, 120.7]) {
        const ticks = niceTicks(duration, 6, start)
        expect(ticks.length).toBeGreaterThanOrEqual(3)
        for (const tick of ticks) {
          const label = start + tick
          const unit = 10 ** Math.floor(Math.log10(duration) - 1)
          expect(label / unit).toBeCloseTo(
            Math.round((label / unit) * 2) / 2,
            6
          )
          expect(tick).toBeGreaterThanOrEqual(0)
          expect(tick).toBeLessThanOrEqual(duration)
        }
      }
    }
  })

  it("gives one round tick rather than the window's own edges on a very narrow ruler", () => {
    const ticks = niceTicks(window, 2, origin)
    expect(ticks.map((tick) => origin + tick)).toEqual([10])
  })
})

describe("niceTicks on a narrow ruler", () => {
  it("keeps three round ticks when only three fit", () => {
    expect(niceTicks(1000, 3)).toEqual([0, 500, 1000])
  })

  it("keeps two round ticks when only two fit", () => {
    expect(niceTicks(1630, 2)).toEqual([0, 1000])
    expect(niceTicks(7, 2)).toEqual([0, 5])
  })

  it("never gives more ticks than fit", () => {
    for (const limit of [2, 3, 4, 5, 6]) {
      for (const duration of [0.37, 3, 12, 99, 312.4, 1630, 4321, 61_000]) {
        expect(niceTicks(duration, limit).length).toBeLessThanOrEqual(limit)
      }
    }
  })
})

describe("tickLimit", () => {
  it("fits one label per 64 px of ruler", () => {
    expect(tickLimit(640)).toBe(10)
    expect(tickLimit(320)).toBe(5)
  })

  it("keeps the start and the end on a ruler too short for more", () => {
    expect(tickLimit(166)).toBe(2)
    expect(tickLimit(0)).toBe(2)
  })
})

describe("tickAnchor", () => {
  it("anchors the first and the last tick to the ruler's ends", () => {
    expect(tickAnchor(0, 1000, 400)).toBe("start")
    expect(tickAnchor(1000, 1000, 400)).toBe("end")
  })

  it("centres a tick with room for its label on both sides", () => {
    expect(tickAnchor(500, 1000, 400)).toBe("middle")
    expect(tickAnchor(250, 1000, 400)).toBe("middle")
  })

  it("pulls a tick whose label would run past the end back to the end", () => {
    expect(tickAnchor(125, 129.9, 350)).toBe("end")
  })

  it("pulls a tick whose label would run past the start back to the start", () => {
    expect(tickAnchor(10, 1000, 400)).toBe("start")
  })

  it("centres a tick on a ruler not measured yet", () => {
    expect(tickAnchor(500, 1000, 0)).toBe("middle")
  })
})
