import { describe, expect, it } from "vitest"

import { isRelativeRange, liveRefreshMs } from "./live-refresh"

describe("liveRefreshMs", () => {
  it.each([
    ["the last 15 minutes", { from: "now-15m", to: "now" }],
    ["today so far", { from: "now/d", to: "now" }],
    ["yesterday", { from: "now-1d/d", to: "now-1d/d" }],
  ])("refreshes every 10 seconds over %s", (_name, range) => {
    expect(liveRefreshMs(true, range)).toBe(10_000)
  })

  it.each([
    ["epoch milliseconds", { from: "1731600000000", to: "1731603600000" }],
    ["dates", { from: "2026-11-14T10:00:00Z", to: "2026-11-14T11:00:00Z" }],
    ["an absolute start", { from: "2026-11-14T10:00:00Z", to: "now" }],
    ["an absolute end", { from: "now-1h", to: "1731603600000" }],
  ])("does not refresh over %s", (_name, range) => {
    expect(liveRefreshMs(true, range)).toBe(false)
  })

  it("does not refresh while Live is off", () => {
    expect(liveRefreshMs(false, { from: "now-15m", to: "now" })).toBe(false)
  })
})

describe("isRelativeRange", () => {
  it("rejects a value that only starts like now", () => {
    expect(isRelativeRange({ from: "nowhere", to: "now" })).toBe(false)
  })
})
