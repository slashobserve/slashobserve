import { describe, expect, test } from "vitest"

import {
  dashboardKeys,
  datasourceKeys,
  editorKeys,
  findingKeys,
  observeKeys,
  savedSearchKeys,
} from "./keys"

function isPrefix(prefix: readonly unknown[], key: readonly unknown[]) {
  return prefix.every((part, index) => part === key[index])
}

describe("dashboardKeys", () => {
  test("history is not under the versions subtree", () => {
    const history = dashboardKeys.history("d1")
    const versions = ["dashboards", "d1", "versions"]
    expect(isPrefix(versions, history)).toBe(false)
  })

  test("a single revision is not under the revisions list subtree", () => {
    expect(
      isPrefix(
        dashboardKeys.revisions("d1"),
        dashboardKeys.revision("d1", "r1")
      )
    ).toBe(false)
  })

  test("titles and the list do not contain each other", () => {
    expect(isPrefix(dashboardKeys.list(), dashboardKeys.titles())).toBe(false)
    expect(isPrefix(dashboardKeys.titles(), dashboardKeys.list())).toBe(false)
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
      dashboardKeys.revision("d1", "r1"),
    ]
    for (const key of keys) {
      expect(isPrefix(dashboardKeys.all, key)).toBe(true)
    }
  })
})

describe("filtered dashboard lists", () => {
  test("every filtered list is under the list key", () => {
    expect(
      isPrefix(dashboardKeys.list(), dashboardKeys.filtered({ q: "x" }, 1))
    ).toBe(true)
    expect(isPrefix(dashboardKeys.list(), dashboardKeys.filtered({}, 3))).toBe(
      true
    )
  })

  test("each page has its own key", () => {
    expect(dashboardKeys.filtered({}, 1)).not.toEqual(
      dashboardKeys.filtered({}, 2)
    )
  })
})

describe("observeKeys", () => {
  test("the caller's requests share the mineAll root", () => {
    for (const filters of [
      {},
      { source: "observe" as const },
      { q: "latency", pinned: true, dashboardId: "d1" },
    ]) {
      expect(
        isPrefix(observeKeys.mineAll(), observeKeys.mine(filters, 20))
      ).toBe(true)
    }
  })

  test("the caller's requests differ by filters and by limit", () => {
    expect(observeKeys.mine({}, 5)).not.toEqual(observeKeys.mine({}, 20))
    expect(observeKeys.mine({}, 5)).not.toEqual(
      observeKeys.mine({ source: "observe" }, 5)
    )
    expect(observeKeys.mine({ q: "a" }, 5)).not.toEqual(
      observeKeys.mine({ q: "b" }, 5)
    )
    expect(observeKeys.mine({ dashboardId: "d1" }, 5)).not.toEqual(
      observeKeys.mine({ dashboardId: "d2" }, 5)
    )
    expect(observeKeys.mine({ q: "a", pinned: true }, 5)).toEqual(
      observeKeys.mine({ q: "a", pinned: true }, 5)
    )
  })

  test("a single request is not under the caller's requests", () => {
    expect(isPrefix(observeKeys.mineAll(), observeKeys.request("x"))).toBe(
      false
    )
  })
})

describe("datasourceKeys", () => {
  test("metric, label and label value names sit under the datasource root", () => {
    expect(
      isPrefix(datasourceKeys.all, datasourceKeys.metrics("prom", ""))
    ).toBe(true)
    expect(
      isPrefix(datasourceKeys.all, datasourceKeys.labels("prom", {}))
    ).toBe(true)
    expect(
      isPrefix(
        datasourceKeys.all,
        datasourceKeys.labelValues("prom", "pod", { metric: "up" })
      )
    ).toBe(true)
  })

  test("label names differ by datasource and by params", () => {
    expect(datasourceKeys.labels("prom", { metric: "up" })).not.toEqual(
      datasourceKeys.labels("prom", { metric: "down" })
    )
    expect(datasourceKeys.labels("prom", {})).not.toEqual(
      datasourceKeys.labels("ch", {})
    )
  })
})

describe("editorKeys", () => {
  test("a preview is not under the dashboard root", () => {
    const key = editorKeys.preview("d1", 3, "p1", {
      panel: "{}",
      selections: {},
    })
    expect(isPrefix(dashboardKeys.all, key)).toBe(false)
  })

  test("a suggestion is not under the dashboard root", () => {
    expect(isPrefix(dashboardKeys.all, editorKeys.suggestion("d", "s"))).toBe(
      false
    )
  })
})

describe("findingKeys", () => {
  test("every finding key sits under the findings prefix", () => {
    for (const key of [
      findingKeys.one("f1"),
      findingKeys.forDashboard("d1"),
      findingKeys.panelData("f1", "p1", { from: "now-1h", to: "now" }),
    ]) {
      expect(isPrefix(findingKeys.all, key)).toBe(true)
    }
  })

  test("panel data sits under its finding", () => {
    expect(
      isPrefix(findingKeys.one("f1"), findingKeys.panelData("f1", "p1", {}))
    ).toBe(true)
    expect(
      isPrefix(findingKeys.one("f2"), findingKeys.panelData("f1", "p1", {}))
    ).toBe(false)
  })
})

describe("savedSearchKeys", () => {
  test("a list sits under the saved searches prefix", () => {
    expect(
      isPrefix(
        savedSearchKeys.all,
        savedSearchKeys.list({ kind: "logs", folderId: "f1" })
      )
    ).toBe(true)
  })

  test("lists with different params are different keys", () => {
    expect(savedSearchKeys.list({ kind: "logs" })).not.toEqual(
      savedSearchKeys.list({ kind: "traces" })
    )
  })
})
