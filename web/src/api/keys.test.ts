import { describe, expect, test } from "vitest"

import { dashboardKeys } from "./keys"

function isPrefix(prefix: readonly unknown[], key: readonly unknown[]) {
  return prefix.every((part, index) => part === key[index])
}

describe("dashboardKeys", () => {
  test("history is not under the versions subtree", () => {
    const history = dashboardKeys.history("d1")
    const versions = ["dashboards", "d1", "versions"]
    expect(isPrefix(versions, history)).toBe(false)
  })

  test("version, panel and variable keys share the version prefix", () => {
    const params = { selections: {} }
    const version = dashboardKeys.version("d1", 3)
    expect(isPrefix(version, dashboardKeys.panels("d1", 3))).toBe(true)
    expect(
      isPrefix(version, dashboardKeys.panelData("d1", 3, "p", params))
    ).toBe(true)
    expect(
      isPrefix(version, dashboardKeys.variableValues("d1", 3, "pod", params))
    ).toBe(true)
    expect(isPrefix(version, dashboardKeys.version("d1", 4))).toBe(false)
  })

  test("every dashboard key sits under the shared root", () => {
    const keys = [
      dashboardKeys.list(),
      dashboardKeys.latest("d1"),
      dashboardKeys.history("d1"),
      dashboardKeys.diff("d1", 1, 2),
      dashboardKeys.comments("d1"),
      dashboardKeys.revisions("d1"),
    ]
    for (const key of keys) {
      expect(isPrefix(dashboardKeys.all, key)).toBe(true)
    }
  })
})
