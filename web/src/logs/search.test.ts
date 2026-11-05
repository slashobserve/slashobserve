import { describe, expect, it } from "vitest"

import { logsTimeRange, validateLogsSearch } from "./search"

describe("validateLogsSearch", () => {
  it("keeps a valid search", () => {
    const search = {
      ds: "loki",
      q: '{service_name="checkout"}',
      from: "now-6h",
      to: "1760000000000",
    }
    expect(validateLogsSearch(search)).toEqual(search)
  })

  it("lists every key so a dropped one overrides the raw URL", () => {
    expect(validateLogsSearch({})).toEqual({
      ds: undefined,
      q: undefined,
      from: undefined,
      to: undefined,
    })
  })

  it.each([
    ["starts with a dash", "-loki"],
    ["has a slash", "lo/ki"],
    ["is too long", "a".repeat(41)],
  ])("drops a datasource uid that %s", (_, ds) => {
    expect(validateLogsSearch({ ds }).ds).toBeUndefined()
  })

  it.each([
    ["is empty", ""],
    ["is over 8000 characters", "x".repeat(8001)],
    ["holds a NUL", '{a="b"}\0'],
  ])("drops a query that %s", (_, q) => {
    expect(validateLogsSearch({ q }).q).toBeUndefined()
  })

  it("keeps the first of repeated values", () => {
    expect(validateLogsSearch({ ds: ["a", "b"] }).ds).toBe("a")
  })

  it.each([
    ["only from", { from: "now-1h" }],
    ["only to", { to: "now" }],
    ["an unreadable from", { from: "yesterday", to: "now" }],
  ])("drops both bounds for %s", (_, raw) => {
    const search = validateLogsSearch(raw)
    expect(search.from).toBeUndefined()
    expect(search.to).toBeUndefined()
  })
})

describe("logsTimeRange", () => {
  it("defaults to the last hour", () => {
    expect(logsTimeRange({})).toEqual({ from: "now-1h", to: "now" })
  })

  it("uses the range in the search", () => {
    expect(logsTimeRange({ from: "now-2d", to: "now-1d" })).toEqual({
      from: "now-2d",
      to: "now-1d",
    })
  })
})
