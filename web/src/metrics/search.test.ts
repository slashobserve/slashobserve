import { describe, expect, it } from "vitest"

import {
  metricsTimeRange,
  modeOf,
  searchAfter,
  validateMetricsEntrySearch,
  validateMetricsSearch,
} from "./search"

describe("validateMetricsSearch", () => {
  it("keeps a saved search id and drops anything else", () => {
    const id = "5f0c1e0e-0000-4000-8000-000000000001"
    expect(validateMetricsSearch({ saved: id }).saved).toBe(id)
    expect(validateMetricsSearch({ saved: "not-an-id" }).saved).toBeUndefined()
  })

  it("keeps a valid search", () => {
    const search = {
      q: "sum(rate(http_requests_total[5m]))",
      from: "now-6h",
      to: "now",
      mode: "table",
    }
    expect(validateMetricsSearch(search)).toMatchObject(search)
  })

  it("lists every key so a dropped one overrides the raw URL", () => {
    expect(Object.keys(validateMetricsSearch({})).sort()).toEqual(
      ["ask", "from", "mode", "observation", "q", "saved", "to"].sort()
    )
  })

  it("drops any mode but table", () => {
    expect(validateMetricsSearch({ mode: "graph" }).mode).toBeUndefined()
    expect(validateMetricsSearch({ mode: "bogus" }).mode).toBeUndefined()
  })

  it("keeps the datasource only on the entry search", () => {
    expect(validateMetricsEntrySearch({ ds: "prom" }).ds).toBe("prom")
    expect(validateMetricsEntrySearch({ ds: "../x" }).ds).toBeUndefined()
    expect("ds" in validateMetricsSearch({ ds: "prom" })).toBe(false)
  })
})

describe("metricsTimeRange", () => {
  it("defaults to the last hour", () => {
    expect(metricsTimeRange({})).toEqual({ from: "now-1h", to: "now" })
  })

  it("uses the range in the search", () => {
    expect(metricsTimeRange({ from: "now-1d", to: "now-1h" })).toEqual({
      from: "now-1d",
      to: "now-1h",
    })
  })
})

describe("modeOf", () => {
  it("reads graph when the address has no mode", () => {
    expect(modeOf({})).toBe("graph")
    expect(modeOf({ mode: "table" })).toBe("table")
  })
})

describe("searchAfter", () => {
  it("merges the change and clears a key set to undefined", () => {
    expect(
      searchAfter({ q: "up", mode: "table" }, { mode: undefined, from: "x" })
    ).toEqual({ q: "up", mode: undefined, from: "x" })
  })
})

describe("the keys a redirect carries to the metrics page", () => {
  it("are the ones the page reads and no others", () => {
    const kept = validateMetricsSearch({
      foo: "1",
      page: "2",
      q: "up",
      from: "now-1h",
      to: "now",
      mode: "table",
      ds: "prom",
    })
    expect(Object.keys(kept)).not.toContain("foo")
    expect(Object.keys(kept)).not.toContain("page")
    expect(Object.keys(kept)).not.toContain("ds")
    expect(kept).toMatchObject({
      q: "up",
      from: "now-1h",
      to: "now",
      mode: "table",
    })
  })
})
