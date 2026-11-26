import { describe, expect, it } from "vitest"

import { calendarMonthOf, readDraft } from "./time-draft"

const unreadableTime = expect.stringMatching(
  /^Enter a time like now-6h, now\/d or \d{4}-\d\d-\d\d \d\d:\d\d\.$/
)

describe("readDraft", () => {
  it("returns the parsed range", () => {
    expect(readDraft("now-6h", "now", undefined)).toEqual({
      range: { from: "now-6h", to: "now" },
    })
  })

  it("blames From for a time it cannot read, with the reason", () => {
    expect(readDraft("yesterday", "now", undefined).problem).toEqual({
      field: "from",
      message: unreadableTime,
    })
    expect(readDraft("1999-01-01 00:00", "now", undefined).problem).toEqual({
      field: "from",
      message: "Times before the year 2000 are not accepted.",
    })
  })

  it("blames To for a time it cannot read", () => {
    expect(readDraft("now-1h", "later", undefined).problem?.field).toBe("to")
  })

  it("blames From when the range is longer than 400 days", () => {
    expect(readDraft("1700000000", "now", undefined).problem).toEqual({
      field: "from",
      message: "The range is longer than 400 days. Choose a From closer to To.",
    })
  })

  it("blames From when it is not earlier than To", () => {
    expect(
      readDraft("2026-10-17T10:00:00Z", "2026-10-17T09:00:00Z", undefined)
        .problem
    ).toEqual({ field: "from", message: "From must be earlier than To." })
  })
})

describe("calendarMonthOf", () => {
  const now = Date.UTC(2026, 10, 18, 12)

  it("opens on the month of an absolute time in the zone", () => {
    expect(calendarMonthOf("2026-03-31 23:30", "UTC", now)).toEqual({
      year: 2026,
      month: 2,
    })
    expect(calendarMonthOf("2026-03-31 23:30", "Asia/Kolkata", now)).toEqual({
      year: 2026,
      month: 2,
    })
  })

  it("opens on the month of a relative time", () => {
    expect(calendarMonthOf("now-30d", "UTC", now)).toEqual({
      year: 2026,
      month: 9,
    })
  })

  it("opens on the current month when the text cannot be read", () => {
    expect(calendarMonthOf("soon", "UTC", now)).toEqual({
      year: 2026,
      month: 10,
    })
  })
})
