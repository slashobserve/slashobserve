import { describe, expect, it } from "vitest"

import { zonedPlotDate } from "./plot-zone"

describe("zonedPlotDate", () => {
  it("leaves uPlot on the browser's clock without a zone", () => {
    expect(zonedPlotDate(undefined)).toEqual({})
  })

  it("moves a half-hour zone's whole hour onto a clock hour", () => {
    const { tzDate } = zonedPlotDate("Asia/Kolkata")
    const wholeHour = tzDate!(Date.UTC(2026, 0, 5, 0, 30, 0) / 1000)
    expect([wholeHour.getHours(), wholeHour.getMinutes()]).toEqual([6, 0])
    const utcHour = tzDate!(Date.UTC(2026, 0, 5, 0, 0, 0) / 1000)
    expect([utcHour.getHours(), utcHour.getMinutes()]).toEqual([5, 30])
  })

  it("follows the zone across its daylight saving change", () => {
    const { tzDate } = zonedPlotDate("America/New_York")
    const before = tzDate!(Date.UTC(2026, 2, 8, 6, 59, 59) / 1000)
    const after = tzDate!(Date.UTC(2026, 2, 8, 7, 0, 0) / 1000)
    expect(before.getHours()).toBe(1)
    expect(after.getHours()).toBe(3)
  })
})
