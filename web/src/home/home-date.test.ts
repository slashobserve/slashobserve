import { describe, expect, it } from "vitest"

import { homeDate } from "./home-date"

describe("homeDate", () => {
  it("names the weekday, then the day and month", () => {
    expect(homeDate(new Date(2026, 10, 13), "en-GB")).toBe(
      "Friday, 13 November"
    )
  })

  it("puts the month first where the locale does", () => {
    expect(homeDate(new Date(2026, 10, 13), "en-US")).toBe(
      "Friday, November 13"
    )
  })

  it("does not pad a single-digit day", () => {
    expect(homeDate(new Date(2026, 0, 2), "en-GB")).toBe("Friday, 2 January")
  })
})
