import { describe, expect, it } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"

import {
  backFromSearch,
  dashboardSearchFromBack,
  panelEditorShowsNewerVersion,
  searchFromState,
  validateBack,
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
      edit: "query",
      panel: "p_rps",
      junk: "y",
      "var-1bad": "z",
    })
    expect(state).toEqual({
      from: "now-1h",
      version: 3,
      panel: "p_rps",
      edit: "query",
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
      edit: "other",
      tab: "other",
    })
    expect(state).toEqual({ vars: {} })
  })

  it("keeps a variable whose only value is empty as one empty value", () => {
    expect(validateDashboardSearch({ "var-svc": "" }).vars).toEqual({
      svc: [""],
    })
    expect(validateDashboardSearch({ "var-svc": ["", ""] }).vars).toEqual({
      svc: [""],
    })
    expect(validateDashboardSearch({ "var-svc": ["", "a"] }).vars).toEqual({
      svc: ["a"],
    })
  })

  it("accepts numbers from in-app navigation", () => {
    expect(validateDashboardSearch({ version: 4 }).version).toBe(4)
    expect(validateDashboardSearch({ "var-n": 7 }).vars).toEqual({ n: ["7"] })
  })

  it("keeps edit only as query or json with a panel", () => {
    expect(validateDashboardSearch({ panel: "rps", edit: "query" })).toEqual({
      panel: "rps",
      edit: "query",
      vars: {},
    })
    expect(validateDashboardSearch({ panel: "p", edit: "json" })).toEqual({
      panel: "p",
      edit: "json",
      vars: {},
    })
    expect(validateDashboardSearch({ edit: "query" })).toEqual({ vars: {} })
    expect(validateDashboardSearch({ edit: "json" })).toEqual({ vars: {} })
    expect(validateDashboardSearch({ panel: "rps", edit: "other" })).toEqual({
      panel: "rps",
      vars: {},
    })
  })

  it("keeps share only as 1 and drops it while a panel editor is open", () => {
    expect(validateDashboardSearch({ share: "1" })).toEqual({
      share: true,
      vars: {},
    })
    expect(validateDashboardSearch({ share: "1", panel: "rps" })).toEqual({
      share: true,
      panel: "rps",
      vars: {},
    })
    expect(validateDashboardSearch({ share: "0" })).toEqual({ vars: {} })
    expect(validateDashboardSearch({ share: "true" })).toEqual({ vars: {} })
    expect(
      validateDashboardSearch({ share: "1", panel: "rps", edit: "query" })
    ).toEqual({ panel: "rps", edit: "query", vars: {} })
  })

  it("maps the legacy comments tab to a panel scope", () => {
    expect(validateDashboardSearch({ panel: "rps", tab: "comments" })).toEqual({
      panel: "rps",
      vars: {},
    })
    expect(validateDashboardSearch({ tab: "comments" })).toEqual({ vars: {} })
  })

  it("maps the legacy query tab to the editor", () => {
    expect(validateDashboardSearch({ panel: "rps", tab: "query" })).toEqual({
      panel: "rps",
      edit: "query",
      vars: {},
    })
    expect(validateDashboardSearch({ tab: "query" })).toEqual({ vars: {} })
  })

  it("keeps a valid cursor mode and drops anything else", () => {
    expect(validateDashboardSearch({ cursor: "tooltip" }).cursor).toBe(
      "tooltip"
    )
    expect(validateDashboardSearch({ cursor: "shared" }).cursor).toBeUndefined()
  })

  it("keeps the cursor mode through the back round trip", () => {
    const back = backFromSearch(
      validateDashboardSearch({ cursor: "crosshair" })
    )
    expect(back).toBe("cursor=crosshair")
    expect(validateBack(back)).toBe("cursor=crosshair")
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

  it("never writes the legacy tab", () => {
    const flat = searchFromState(
      validateDashboardSearch({ panel: "rps", tab: "query" })
    )
    expect(flat).toEqual({ panel: "rps", edit: "query" })
  })

  it("writes an empty value as var-<name>= and reads it back", () => {
    const state = validateDashboardSearch({ "var-svc": "" })
    const text = stringifySearch(searchFromState(state))
    expect(text).toBe("?var-svc=")
    expect(validateDashboardSearch(parseSearch(text))).toEqual(state)
  })

  it("writes share as 1 and never together with edit", () => {
    expect(searchFromState({ share: true, panel: "rps", vars: {} })).toEqual({
      panel: "rps",
      share: "1",
    })
    expect(
      searchFromState({ share: true, panel: "rps", edit: "json", vars: {} })
    ).toEqual({ panel: "rps", edit: "json" })
    expect(searchFromState({ share: false, vars: {} })).toEqual({})
  })

  it("round-trips through validation", () => {
    const state = validateDashboardSearch({
      refresh: "5s",
      panel: "rps",
      edit: "query",
      "var-pod": ["a"],
    })
    expect(validateDashboardSearch(searchFromState(state))).toEqual(state)
  })
})

describe("dashboardSearchFromBack", () => {
  const back = "from=now-1h&version=3&panel=rps&edit=query&var-pod=a"

  it("keeps the whole search by default", () => {
    expect(dashboardSearchFromBack(back)).toEqual({
      from: "now-1h",
      version: 3,
      panel: "rps",
      edit: "query",
      "var-pod": ["a"],
    })
  })

  it("drops the version, panel and editor when asked", () => {
    expect(dashboardSearchFromBack(back, { dropVersion: true })).toEqual({
      from: "now-1h",
      "var-pod": ["a"],
    })
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

describe("panelEditorShowsNewerVersion", () => {
  it.each([
    ["no panel editor is open", {}, false],
    ["only a panel is selected", { panel: "p1" }, false],
    ["the query editor is open", { panel: "p1", edit: "query" }, true],
    ["the JSON editor is open", { panel: "p1", edit: "json" }, true],
    [
      "the editor is open on a pinned version",
      { panel: "p1", edit: "query", version: 2 },
      false,
    ],
  ] as const)("when %s returns %s", (_name, search, expected) => {
    expect(panelEditorShowsNewerVersion({ vars: {}, ...search })).toBe(expected)
  })
})
