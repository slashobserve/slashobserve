import { describe, expect, test } from "vitest"

import { filterPalette } from "./palette-items"

const thirty = Array.from({ length: 30 }, (_, index) => ({
  id: `d${index}`,
  title: `Dashboard ${index}`,
}))

describe("filterPalette", () => {
  test("an empty query lists every page, the first 8 dashboards and sign out", () => {
    const groups = filterPalette("", thirty)
    expect(groups.map((group) => group.value)).toEqual([
      "Pages",
      "Dashboards",
      "Account",
    ])
    expect(groups[0]?.items.map((entry) => entry.label)).toEqual([
      "Observe",
      "Dashboards",
      "Datasources",
      "API keys",
      "People",
      "Settings",
    ])
    expect(groups[1]?.items.map((entry) => entry.id)).toEqual(
      thirty.slice(0, 8).map((dashboard) => dashboard.id)
    )
    expect(groups[2]?.items.map((entry) => entry.label)).toEqual(["Sign out"])
  })

  test("a query matching one page omits the empty groups", () => {
    const groups = filterPalette("api", thirty)
    expect(groups).toHaveLength(1)
    expect(groups[0]?.value).toBe("Pages")
    expect(groups[0]?.items.map((entry) => entry.label)).toEqual(["API keys"])
  })

  test("matches titles case-insensitively", () => {
    const groups = filterPalette("CHECK", [
      { id: "a", title: "Checkout health" },
      { id: "b", title: "Billing" },
    ])
    expect(groups).toHaveLength(1)
    expect(groups[0]?.value).toBe("Dashboards")
    expect(groups[0]?.items.map((entry) => entry.id)).toEqual(["a"])
  })

  test("cuts matching dashboards to 20", () => {
    const many = Array.from({ length: 25 }, (_, index) => ({
      id: `m${index}`,
      title: `Match ${index}`,
    }))
    const groups = filterPalette("match", many)
    expect(groups[0]?.value).toBe("Dashboards")
    expect(groups[0]?.items).toHaveLength(20)
  })

  test("finds sign out in the Account group", () => {
    const groups = filterPalette("sign", [])
    expect(groups.map((group) => group.value)).toEqual(["Account"])
    expect(groups[0]?.items[0]).toMatchObject({ kind: "action" })
  })

  test("a query matching nothing returns no groups", () => {
    expect(filterPalette("zzzz-no-match", thirty)).toEqual([])
  })
})
