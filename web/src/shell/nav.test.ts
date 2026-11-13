import { describe, expect, test } from "vitest"

import {
  countLabel,
  isNavItemActive,
  unreadBadge,
  visibleNavItems,
} from "./nav"

describe("isNavItemActive", () => {
  test("Inbox covers its own page only, not the chat or the importer", () => {
    expect(isNavItemActive("/inbox", "/inbox")).toBe(true)
    expect(isNavItemActive("/inbox", "/inbox-x")).toBe(false)
    expect(isNavItemActive("/inbox", "/dashboards")).toBe(false)
    const active = (pathname: string) =>
      visibleNavItems("admin")
        .filter((item) => isNavItemActive(item.to, pathname))
        .map((item) => item.label)
    expect(active("/ask")).toEqual([])
    expect(active("/c/abc")).toEqual([])
    expect(active("/import")).toEqual([])
  })

  test("other items match their path and children only", () => {
    expect(isNavItemActive("/dashboards", "/dashboards")).toBe(true)
    expect(isNavItemActive("/dashboards", "/dashboards-x")).toBe(false)
    expect(isNavItemActive("/settings", "/settings")).toBe(true)
  })

  test("Agents covers its list, an agent's page and the API keys page", () => {
    expect(isNavItemActive("/agents", "/agents")).toBe(true)
    expect(isNavItemActive("/agents", "/agents/grant-abc")).toBe(true)
    expect(isNavItemActive("/agents", "/keys")).toBe(true)
    expect(isNavItemActive("/agents", "/agents-x")).toBe(false)
    expect(isNavItemActive("/agents", "/keysx")).toBe(false)
    expect(isNavItemActive("/settings", "/keys")).toBe(false)
  })

  test("Observations covers its list and a single observation", () => {
    const active = visibleNavItems("viewer")
      .filter((item) =>
        isNavItemActive(item.to, "/o/3f6c1d1e-8a54-4d57-9a3b-2c7d0b5f9e10")
      )
      .map((item) => item.label)
    expect(active).toEqual(["Observations"])
    expect(isNavItemActive("/observations", "/observations")).toBe(true)
    expect(isNavItemActive("/observations", "/observations-x")).toBe(false)
    expect(isNavItemActive("/inbox", "/o/abc")).toBe(false)
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

describe("Home", () => {
  test("is active on the root path only", () => {
    expect(isNavItemActive("/", "/")).toBe(true)
    expect(isNavItemActive("/", "/inbox")).toBe(false)
    expect(isNavItemActive("/", "/o/abc")).toBe(false)
    expect(isNavItemActive("/", "/login")).toBe(false)
  })

  test("no other item is active on the root path", () => {
    const active = visibleNavItems("admin")
      .filter((item) => isNavItemActive(item.to, "/"))
      .map((item) => item.label)
    expect(active).toEqual(["Home"])
  })
})

describe("visibleNavItems", () => {
  const labels = (role: Parameters<typeof visibleNavItems>[0]) =>
    visibleNavItems(role).map((item) => item.label)

  test("an Admin sees all eleven pages and neither the chat nor API keys is one of them", () => {
    expect(labels("admin")).toEqual([
      "Home",
      "Inbox",
      "Observations",
      "Dashboards",
      "Logs",
      "Traces",
      "Agents",
      "Datasources",
      "People",
      "Settings",
    ])
  })

  test.each(["editor", "commenter", "viewer"] as const)(
    "%s sees every page but the Admin ones",
    (role) => {
      expect(labels(role)).toEqual([
        "Home",
        "Inbox",
        "Observations",
        "Dashboards",
        "Logs",
        "Traces",
        "Agents",
        "Settings",
      ])
    }
  )
})

describe("unreadBadge", () => {
  test.each([
    [null, null],
    [0, null],
    [-1, null],
    [1, "1"],
    [42, "42"],
    [99, "99"],
    [100, "99+"],
  ])("%s unread shows %s", (count, shown) => {
    expect(unreadBadge(count)).toBe(shown)
  })
})

describe("countLabel", () => {
  test.each([
    [0, false, null],
    [3, false, "3"],
    [5, true, "5+"],
  ])("%s shown with more=%s is %s", (shown, more, label) => {
    expect(countLabel(shown, more)).toBe(label)
  })
})
