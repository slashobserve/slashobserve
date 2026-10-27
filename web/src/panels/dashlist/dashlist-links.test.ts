import { describe, expect, it } from "vitest"

import { dashlistOptions, dashlistSearch } from "./dashlist-links"

const range = { from: "now-6h", to: "now" }
const vars = { pod: ["a", "b"], env: ["prod"] }

describe("dashlistSearch", () => {
  it("carries nothing by default", () => {
    expect(
      dashlistSearch({ keepTime: false, includeVars: false }, range, vars)
    ).toEqual({})
  })

  it("carries the time range only with keepTime", () => {
    expect(
      dashlistSearch({ keepTime: true, includeVars: false }, range, vars)
    ).toEqual({ from: "now-6h", to: "now" })
  })

  it("carries the variables as var- parameters only with includeVars", () => {
    expect(
      dashlistSearch({ keepTime: false, includeVars: true }, range, vars)
    ).toEqual({ "var-pod": ["a", "b"], "var-env": ["prod"] })
  })

  it("carries both when both are on", () => {
    expect(
      dashlistSearch({ keepTime: true, includeVars: true }, range, vars)
    ).toEqual({
      from: "now-6h",
      to: "now",
      "var-pod": ["a", "b"],
      "var-env": ["prod"],
    })
  })
})

describe("dashlistOptions", () => {
  it("fills the defaults for a panel with no display", () => {
    expect(dashlistOptions(undefined)).toEqual({
      query: "",
      tags: [],
      limit: 10,
      keepTime: false,
      includeVars: false,
    })
  })

  it("trims the search text and keeps the given values", () => {
    expect(
      dashlistOptions({
        query: "  shop ",
        tags: ["demo", "shop"],
        limit: 25,
        keepTime: true,
        includeVars: true,
      })
    ).toEqual({
      query: "shop",
      tags: ["demo", "shop"],
      limit: 25,
      keepTime: true,
      includeVars: true,
    })
  })

  it("keeps the limit inside what the list endpoint accepts", () => {
    expect(dashlistOptions({ limit: 0 }).limit).toBe(1)
    expect(dashlistOptions({ limit: 500 }).limit).toBe(100)
    expect(dashlistOptions({ limit: 2.9 }).limit).toBe(2)
  })
})
