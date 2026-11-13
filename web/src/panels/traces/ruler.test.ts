import { describe, expect, it } from "vitest"

import { niceTicks, tickAnchor, tickLimit } from "./ruler"

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
      const step = ticks[1] / 10 ** Math.floor(Math.log10(ticks[1]))
      expect([1, 2, 2.5, 5]).toContainEqual(Number(step.toFixed(6)))
    }
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
