import { describe, expect, it } from "vitest"

import { dayLabel, nextDayStart, withDayBreaks } from "./day-label"

const now = new Date(2026, 10, 6, 15, 30).getTime()

function iso(...parts: [number, number, number, number?, number?]): string {
  const [year, month, day, hours = 12, minutes = 0] = parts
  return new Date(year, month, day, hours, minutes).toISOString()
}

describe("dayLabel", () => {
  it("reads Today for the current local day", () => {
    expect(dayLabel(iso(2026, 10, 6, 0, 5), now)).toBe("Today")
  })

  it("reads Yesterday for the previous local day", () => {
    expect(dayLabel(iso(2026, 10, 5, 23, 59), now)).toBe("Yesterday")
  })

  it("names the weekday and date earlier this year", () => {
    expect(dayLabel(iso(2026, 10, 4), now)).toBe("Wed, Nov 4")
  })

  it("names the year for another year", () => {
    expect(dayLabel(iso(2025, 10, 4), now)).toBe("Nov 4, 2025")
  })

  it("is empty for a date it cannot read", () => {
    expect(dayLabel("not a date", now)).toBe("")
  })
})

describe("withDayBreaks", () => {
  it("adds a break before the first entry of each day", () => {
    const entries = [
      { key: "a", at: iso(2026, 10, 5, 9) },
      { key: "b", at: iso(2026, 10, 5, 18) },
      { key: "c", at: iso(2026, 10, 6, 8) },
    ]
    expect(withDayBreaks(entries, now)).toEqual([
      { kind: "day", key: "day:2026-11-05", label: "Yesterday" },
      { kind: "entry", entry: entries[0] },
      { kind: "entry", entry: entries[1] },
      { kind: "day", key: "day:2026-11-06", label: "Today" },
      { kind: "entry", entry: entries[2] },
    ])
  })
})

describe("nextDayStart", () => {
  it("is the next local midnight", () => {
    expect(nextDayStart(now)).toBe(new Date(2026, 10, 7).getTime())
  })
})
