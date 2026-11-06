import { describe, expect, it } from "vitest"

import { parseSearch, stringifySearch } from "@/lib/search"

import {
  backFromSearch,
  dashboardSearchFromBack,
  panelEditorShowsNewerVersion,
  parseAnnotationTags,
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

describe("explicit empty variable value", () => {
  it("round-trips through the URL as var-<name>=", () => {
    const text = stringifySearch(searchFromState({ vars: { af: [""] } }))
    expect(text).toBe("?var-af=")
    expect(validateDashboardSearch(parseSearch(text)).vars).toEqual({
      af: [""],
    })
  })

  it("sends var-<name>= on API calls", () => {
    expect(stringifySearch(withVariables({}, { af: [""] }))).toBe("?var-af=")
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

describe("parseAnnotationTags", () => {
  it("keeps a comma-separated list of valid tags in order", () => {
    expect(parseAnnotationTags("deploy, incident,checkout")).toEqual([
      "deploy",
      "incident",
      "checkout",
    ])
  })

  it("drops duplicates", () => {
    expect(parseAnnotationTags("deploy,deploy,incident")).toEqual([
      "deploy",
      "incident",
    ])
  })

  it("drops an uppercase or over-long tag", () => {
    expect(parseAnnotationTags(`Deploy,${"a".repeat(41)},ok`)).toEqual(["ok"])
  })

  it("answers undefined when nothing valid remains", () => {
    expect(parseAnnotationTags("")).toBeUndefined()
    expect(parseAnnotationTags(" , ")).toBeUndefined()
    expect(parseAnnotationTags(undefined)).toBeUndefined()
  })

  it("keeps at most ten tags", () => {
    const raw = Array.from({ length: 12 }, (_, i) => `t${i}`).join(",")
    expect(parseAnnotationTags(raw)).toEqual(
      Array.from({ length: 10 }, (_, i) => `t${i}`)
    )
  })

  it("round-trips through the dashboard search", () => {
    const state = validateDashboardSearch(
      parseSearch("?ann=deploy%2Cincident&from=now-6h")
    )
    expect(state.ann).toEqual(["deploy", "incident"])
    const flat = searchFromState(state)
    expect(flat.ann).toBe("deploy,incident")
    expect(validateDashboardSearch(flat)).toEqual(state)
  })
})

describe("viewing keys", () => {
  it("round-trips every viewing key through the URL", () => {
    const state = validateDashboardSearch(
      parseSearch(
        "?kiosk=1&cycle=folder&interval=120&viewPanel=p_rps&inspect=p_err&inspectTab=stats"
      )
    )
    expect(state).toEqual({
      kiosk: true,
      cycle: "folder",
      interval: 120,
      viewPanel: "p_rps",
      inspect: "p_err",
      inspectTab: "stats",
      vars: {},
    })
    expect(validateDashboardSearch(searchFromState(state))).toEqual(state)
  })

  it.each([
    ["5", 10],
    ["10", 10],
    ["600", 600],
    ["3600", 3600],
    ["86400", 3600],
  ])("clamps interval %s to %d seconds", (raw, expected) => {
    expect(
      validateDashboardSearch({ kiosk: "1", cycle: "rows", interval: raw })
        .interval
    ).toBe(expected)
  })

  it.each([
    ["an unknown cycle", { kiosk: "1", cycle: "panels" }, { kiosk: true }],
    ["a cycle without kiosk", { cycle: "rows" }, {}],
    [
      "an interval without a cycle",
      { kiosk: "1", interval: "30" },
      { kiosk: true },
    ],
    [
      "a non-numeric interval",
      { kiosk: "1", cycle: "rows", interval: "fast" },
      { kiosk: true, cycle: "rows" },
    ],
    ["a kiosk value other than on", { kiosk: "0" }, {}],
    ["a malformed panel id", { viewPanel: "1bad", inspect: "x y" }, {}],
    [
      "an unknown inspect tab",
      { inspect: "p", inspectTab: "raw" },
      { inspect: "p" },
    ],
    ["an inspect tab without inspect", { inspectTab: "query" }, {}],
  ])("drops %s", (_name, raw, expected) => {
    expect(validateDashboardSearch(raw)).toEqual({ ...expected, vars: {} })
  })

  it("accepts a bare kiosk key the way Grafana links write it", () => {
    expect(validateDashboardSearch(parseSearch("?kiosk")).kiosk).toBe(true)
    expect(validateDashboardSearch({ kiosk: "true" }).kiosk).toBe(true)
  })

  it("drops view and inspect while a panel editor is open", () => {
    expect(
      validateDashboardSearch({
        panel: "p",
        edit: "query",
        viewPanel: "p",
        inspect: "p",
      })
    ).toEqual({ panel: "p", edit: "query", vars: {} })
    expect(
      searchFromState({
        panel: "p",
        edit: "json",
        viewPanel: "p",
        inspect: "p",
        vars: {},
      })
    ).toEqual({ panel: "p", edit: "json" })
  })

  it("omits the default interval and inspect tab", () => {
    expect(
      searchFromState({
        kiosk: true,
        cycle: "rows",
        interval: 60,
        inspect: "p",
        inspectTab: "data",
        vars: {},
      })
    ).toEqual({ kiosk: "1", cycle: "rows", inspect: "p" })
  })

  it("writes kiosk as 1 and nothing when it is off", () => {
    expect(searchFromState({ kiosk: true, vars: {} })).toEqual({ kiosk: "1" })
    expect(searchFromState({ kiosk: false, cycle: "rows", vars: {} })).toEqual(
      {}
    )
  })
})
