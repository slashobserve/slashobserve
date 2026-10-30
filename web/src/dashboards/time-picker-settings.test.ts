import { describe, expect, it } from "vitest"

import { quickRanges, refreshOptions } from "@/lib/time"

import {
  effectiveRefresh,
  pickerQuickRanges,
  pickerRefreshOptions,
} from "./time-picker-settings"

describe("pickerQuickRanges", () => {
  it("offers the built-in ranges when the dashboard writes none", () => {
    expect(pickerQuickRanges({})).toBe(quickRanges)
    expect(pickerQuickRanges({ timePicker: { hidden: true } })).toBe(
      quickRanges
    )
  })

  it("replaces them with the dashboard's own, named by display", () => {
    expect(
      pickerQuickRanges({
        timePicker: {
          quickRanges: [
            { display: "Last 2 days", from: "now-2d", to: "now" },
            {
              display: "Sprint",
              from: "2026-10-01T00:00:00Z",
              to: "2026-10-15T00:00:00Z",
            },
          ],
        },
      })
    ).toEqual([
      { label: "Last 2 days", from: "now-2d", to: "now" },
      {
        label: "Sprint",
        from: "2026-10-01T00:00:00Z",
        to: "2026-10-15T00:00:00Z",
      },
    ])
  })
})

describe("pickerRefreshOptions", () => {
  it("offers the built-in choices when the dashboard writes none", () => {
    expect(pickerRefreshOptions({})).toBe(refreshOptions)
  })

  it("puts Off first and then the dashboard's own intervals", () => {
    expect(
      pickerRefreshOptions({
        timePicker: { refreshIntervals: ["10s", "1m", "1d"] },
      })
    ).toEqual(["off", "10s", "1m", "1d"])
  })
})

describe("effectiveRefresh", () => {
  it("prefers the URL, then the saved refresh", () => {
    expect(effectiveRefresh("1m", { refresh: "30s", liveNow: true })).toBe("1m")
    expect(effectiveRefresh(undefined, { refresh: "30s", liveNow: true })).toBe(
      "30s"
    )
  })

  it("refreshes every few seconds for live now until a choice is made", () => {
    expect(effectiveRefresh(undefined, { liveNow: true })).toBe("5s")
    expect(effectiveRefresh("off", { liveNow: true })).toBe("off")
  })

  it("is off with no setting at all", () => {
    expect(effectiveRefresh(undefined, {})).toBe("off")
  })
})
