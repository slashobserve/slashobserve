import { describe, expect, test } from "vitest"

import { isNavItemActive, navItems } from "./nav"

describe("isNavItemActive", () => {
  test("Observe covers the chat, the importer and requests", () => {
    expect(isNavItemActive("/", "/")).toBe(true)
    expect(isNavItemActive("/", "/observe/abc")).toBe(true)
    expect(isNavItemActive("/", "/import")).toBe(true)
    expect(isNavItemActive("/", "/dashboards")).toBe(false)
  })

  test("other items match their path and children only", () => {
    expect(isNavItemActive("/dashboards", "/dashboards")).toBe(true)
    expect(isNavItemActive("/dashboards", "/dashboards-x")).toBe(false)
    expect(isNavItemActive("/keys", "/keys")).toBe(true)
    expect(isNavItemActive("/settings", "/settings")).toBe(true)
  })

  test("no item is active on the login page", () => {
    for (const item of navItems) {
      expect(isNavItemActive(item.to, "/login")).toBe(false)
    }
  })
})
