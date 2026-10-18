import { describe, expect, it } from "vitest"

import {
  searchFromState,
  validateDashboardSearch,
  withVariables,
} from "./search"

describe("validateDashboardSearch", () => {
  it("keeps valid keys and drops everything else", () => {
    const state = validateDashboardSearch({
      from: "now-1h",
      "var-pod": ["a", "b"],
      "var-x": "1",
      version: "3",
      tab: "query",
      panel: "p_rps",
      junk: "y",
      "var-1bad": "z",
    })
    expect(state).toEqual({
      from: "now-1h",
      version: 3,
      panel: "p_rps",
      tab: "query",
      vars: { pod: ["a", "b"], x: ["1"] },
    })
  })

  it("rejects malformed values", () => {
    const state = validateDashboardSearch({
      from: "yesterday",
      to: "nope",
      refresh: "soon",
      version: "0",
      panel: "1bad",
      tab: "other",
      "var-empty": "",
    })
    expect(state).toEqual({ vars: {} })
  })

  it("accepts numbers from in-app navigation", () => {
    expect(validateDashboardSearch({ version: 4 }).version).toBe(4)
    expect(validateDashboardSearch({ "var-n": 7 }).vars).toEqual({ n: ["7"] })
  })

  it("allows the comments tab without a panel but not the query tab", () => {
    expect(validateDashboardSearch({ tab: "comments" }).tab).toBe("comments")
    expect(validateDashboardSearch({ tab: "query" }).tab).toBeUndefined()
  })

  it("accepts the refresh options and custom intervals", () => {
    expect(validateDashboardSearch({ refresh: "off" }).refresh).toBe("off")
    expect(validateDashboardSearch({ refresh: "45s" }).refresh).toBe("45s")
  })

  it("takes the first value of a repeated scalar key", () => {
    expect(validateDashboardSearch({ from: ["now-1h", "now-2h"] }).from).toBe(
      "now-1h"
    )
  })
})

describe("searchFromState", () => {
  it("flattens variables to var- keys in a stable order", () => {
    const flat = searchFromState({
      from: "now-1h",
      to: "now",
      version: 3,
      vars: { pod: ["a", "b"], x: ["1"] },
    })
    expect(flat).toEqual({
      from: "now-1h",
      to: "now",
      version: 3,
      "var-pod": ["a", "b"],
      "var-x": ["1"],
    })
    expect(Object.keys(flat)).toEqual([
      "from",
      "to",
      "version",
      "var-pod",
      "var-x",
    ])
  })

  it("round-trips through validation", () => {
    const state = validateDashboardSearch({
      refresh: "5s",
      panel: "rps",
      "var-pod": ["a"],
    })
    expect(validateDashboardSearch(searchFromState(state))).toEqual(state)
  })
})

describe("withVariables", () => {
  it("adds one repeated var- parameter per variable", () => {
    expect(withVariables({ from: "now-1h" }, { pod: ["a", "b"] })).toEqual({
      from: "now-1h",
      "var-pod": ["a", "b"],
    })
  })
})
