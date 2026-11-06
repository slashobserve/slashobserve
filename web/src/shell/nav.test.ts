import { describe, expect, test } from "vitest"

import { isNavItemActive, visibleNavItems } from "./nav"

describe("isNavItemActive", () => {
  test("Observe covers the chat, the importer and requests", () => {
    expect(isNavItemActive("/", "/")).toBe(true)
    expect(isNavItemActive("/", "/c/abc")).toBe(true)
    expect(isNavItemActive("/", "/import")).toBe(true)
    expect(isNavItemActive("/", "/dashboards")).toBe(false)
  })

  test("other items match their path and children only", () => {
    expect(isNavItemActive("/dashboards", "/dashboards")).toBe(true)
    expect(isNavItemActive("/dashboards", "/dashboards-x")).toBe(false)
    expect(isNavItemActive("/keys", "/keys")).toBe(true)
    expect(isNavItemActive("/settings", "/settings")).toBe(true)
  })

  test("Traces also covers a single trace", () => {
    const active = visibleNavItems("admin")
      .filter((item) => isNavItemActive(item.to, "/t/abc"))
      .map((item) => item.label)
    expect(active).toEqual(["Traces"])
    expect(isNavItemActive("/traces", "/traces")).toBe(true)
    expect(isNavItemActive("/logs", "/logs")).toBe(true)
  })

  test("no item is active on the login page", () => {
    for (const item of visibleNavItems("admin")) {
      expect(isNavItemActive(item.to, "/login")).toBe(false)
    }
  })
})

describe("visibleNavItems", () => {
  const labels = (role: Parameters<typeof visibleNavItems>[0]) =>
    visibleNavItems(role).map((item) => item.label)

  test("an Admin sees all nine pages", () => {
    expect(labels("admin")).toEqual([
      "Observe",
      "Dashboards",
      "Health",
      "Logs",
      "Traces",
      "Datasources",
      "API keys",
      "People",
      "Settings",
    ])
  })

  test.each(["editor", "commenter", "viewer"] as const)(
    "%s sees every page but the Admin ones",
    (role) => {
      expect(labels(role)).toEqual([
        "Observe",
        "Dashboards",
        "Health",
        "Logs",
        "Traces",
        "Settings",
      ])
    }
  )
})
