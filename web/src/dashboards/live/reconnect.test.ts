import { describe, expect, it } from "vitest"

import { backoffDelay, shouldCheckDashboard } from "./reconnect"

describe("backoffDelay", () => {
  it("spreads each attempt between half and all of the base delay", () => {
    expect(backoffDelay(0, () => 0)).toBe(500)
    expect(backoffDelay(0, () => 0.5)).toBe(750)
    expect(backoffDelay(0, () => 1)).toBe(1_000)
    expect(backoffDelay(3, () => 0)).toBe(4_000)
    expect(backoffDelay(3, () => 1)).toBe(8_000)
  })

  it("caps the base delay at 30 seconds", () => {
    expect(backoffDelay(5, () => 1)).toBe(30_000)
    expect(backoffDelay(60, () => 0)).toBe(15_000)
    expect(backoffDelay(60, () => 1)).toBe(30_000)
  })

  it("stays inside the jitter window with the real random source", () => {
    for (let attempt = 0; attempt < 8; attempt++) {
      const base = Math.min(1_000 * 2 ** attempt, 30_000)
      for (let run = 0; run < 50; run++) {
        const delay = backoffDelay(attempt)
        expect(delay).toBeGreaterThanOrEqual(base / 2)
        expect(delay).toBeLessThanOrEqual(base)
      }
    }
  })
})

describe("shouldCheckDashboard", () => {
  it("checks on every third consecutive failure", () => {
    const checked = Array.from({ length: 10 }, (_, index) => index).filter(
      shouldCheckDashboard
    )
    expect(checked).toEqual([3, 6, 9])
  })
})
