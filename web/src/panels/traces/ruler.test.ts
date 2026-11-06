import { describe, expect, it } from "vitest"

import { niceTicks } from "./ruler"

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
