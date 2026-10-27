import { describe, expect, it } from "vitest"

import { timeOverrideLabel } from "./time-override"

describe("timeOverrideLabel", () => {
  it("has no label without an override", () => {
    expect(timeOverrideLabel({})).toBeNull()
    expect(timeOverrideLabel({ timeFrom: "", timeShift: "" })).toBeNull()
  })

  it("describes timeFrom alone", () => {
    expect(timeOverrideLabel({ timeFrom: "now-1h" })).toEqual({
      text: "Last 1 hour",
      title: "timeFrom now-1h",
    })
  })

  it("describes timeShift alone", () => {
    expect(timeOverrideLabel({ timeShift: "1d" })).toEqual({
      text: "Shifted 1 day",
      title: "timeShift 1d",
    })
  })

  it("describes both", () => {
    expect(timeOverrideLabel({ timeFrom: "now-1h", timeShift: "1d" })).toEqual({
      text: "Last 1 hour, shifted 1 day",
      title: "timeFrom now-1h, timeShift 1d",
    })
  })

  it.each([
    ["30s", "Shifted 30 seconds"],
    ["1m", "Shifted 1 minute"],
    ["2h", "Shifted 2 hours"],
    ["1w", "Shifted 1 week"],
    ["3M", "Shifted 3 months"],
    ["1y", "Shifted 1 year"],
  ])("writes the unit of a %s shift in words", (shift, text) => {
    expect(timeOverrideLabel({ timeShift: shift })?.text).toBe(text)
  })

  it("pluralises a timeFrom outside the quick ranges", () => {
    expect(timeOverrideLabel({ timeFrom: "now-90d" })?.text).toBe(
      "Last 90 days"
    )
    expect(timeOverrideLabel({ timeFrom: "now-1y" })?.text).toBe("Last 1 year")
  })
})
