import { describe, expect, it } from "vitest"

import { formatDate, parseDate } from "./date-format"

const noon = Date.UTC(2026, 9, 7, 15, 4, 9, 7)

describe("formatDate", () => {
  it.each([
    ["YYYY-MM-DD", "2026-10-07"],
    ["YY/M/D", "26/10/7"],
    ["HH:mm:ss.SSS", "15:04:09.007"],
    ["hh:mm A", "03:04 PM"],
    ["h:mm a", "3:04 pm"],
    ["H", "15"],
    ["no tokens", "no tokens"],
  ])("writes %s as %s in UTC", (format, expected) => {
    expect(formatDate(noon, format, true)).toBe(expected)
  })

  it("writes midnight and noon on a 12 hour clock", () => {
    expect(formatDate(Date.UTC(2026, 0, 1, 0, 0), "h A", true)).toBe("12 AM")
    expect(formatDate(Date.UTC(2026, 0, 1, 12, 0), "h A", true)).toBe("12 PM")
  })

  it("uses local time unless asked for UTC", () => {
    const local = new Date(2026, 9, 7, 15, 4, 9).getTime()
    expect(formatDate(local, "YYYY-MM-DD HH:mm:ss")).toBe("2026-10-07 15:04:09")
  })

  it("returns the input as text when the time is not valid", () => {
    expect(formatDate(Number.NaN, "YYYY")).toBe("NaN")
  })
})

describe("parseDate", () => {
  it("reads what formatDate writes", () => {
    expect(
      parseDate("2026-10-07 15:04:09.007", "YYYY-MM-DD HH:mm:ss.SSS", true)
    ).toBe(noon)
  })

  it("reads a 12 hour clock", () => {
    expect(parseDate("10/07/26 03:04 PM", "MM/DD/YY hh:mm A", true)).toBe(
      Date.UTC(2026, 9, 7, 15, 4)
    )
    expect(parseDate("12:30 am", "hh:mm a", true)).toBe(
      Date.UTC(1970, 0, 1, 0, 30)
    )
  })

  it.each([
    ["2026-10-07", "YYYY/MM/DD"],
    ["2026-02-30", "YYYY-MM-DD"],
    ["not a date", "YYYY-MM-DD"],
  ])("refuses %s for %s", (text, format) => {
    expect(parseDate(text, format, true)).toBeNull()
  })

  it("escapes the format's own punctuation", () => {
    expect(parseDate("2026.10.07", "YYYY.MM.DD", true)).toBe(
      Date.UTC(2026, 9, 7)
    )
    expect(parseDate("2026x10x07", "YYYY.MM.DD", true)).toBeNull()
  })
})
