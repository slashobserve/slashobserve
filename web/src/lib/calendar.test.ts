import { describe, expect, it } from "vitest"

import {
  dayText,
  monthTitle,
  monthWeeks,
  pickDay,
  selectedSpan,
  shiftMonth,
} from "./calendar"

describe("monthWeeks", () => {
  it("starts weeks on Monday and pads with the neighbouring months", () => {
    const weeks = monthWeeks(2026, 9)
    expect(weeks.map((week) => week.length)).toEqual([7, 7, 7, 7, 7])
    expect(dayText(weeks[0]![0]!)).toBe("2026-09-28")
    expect(dayText(weeks[0]![3]!)).toBe("2026-10-01")
    expect(dayText(weeks[4]![6]!)).toBe("2026-11-01")
  })

  it("uses six rows when the month needs them", () => {
    expect(monthWeeks(2026, 10)).toHaveLength(6)
  })

  it("places February 2027 in four rows", () => {
    expect(monthWeeks(2027, 1)).toHaveLength(4)
  })
})

describe("shiftMonth", () => {
  it("rolls over a year in both directions", () => {
    expect(shiftMonth(2026, 11, 1)).toEqual({ year: 2027, month: 0 })
    expect(shiftMonth(2026, 0, -1)).toEqual({ year: 2025, month: 11 })
    expect(shiftMonth(2026, 5, -17)).toEqual({ year: 2025, month: 0 })
  })
})

describe("monthTitle", () => {
  it("names the month and year", () => {
    expect(monthTitle(2026, 9)).toBe("October 2026")
  })
})

describe("pickDay", () => {
  it("starts a selection, then closes it in date order", () => {
    const first = pickDay(null, "2026-10-17")
    expect(first).toEqual({ start: "2026-10-17", end: null })
    expect(pickDay(first, "2026-10-20")).toEqual({
      start: "2026-10-17",
      end: "2026-10-20",
    })
    expect(pickDay(first, "2026-10-10")).toEqual({
      start: "2026-10-10",
      end: "2026-10-17",
    })
  })

  it("starts over after a closed selection", () => {
    const closed = { start: "2026-10-17", end: "2026-10-20" }
    expect(pickDay(closed, "2026-11-02")).toEqual({
      start: "2026-11-02",
      end: null,
    })
  })
})

describe("selectedSpan", () => {
  it("treats a single picked day as a one day span", () => {
    expect(selectedSpan({ start: "2026-10-17", end: null })).toEqual({
      first: "2026-10-17",
      last: "2026-10-17",
    })
    expect(selectedSpan(null)).toBeNull()
  })
})
