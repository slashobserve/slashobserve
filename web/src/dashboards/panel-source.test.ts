import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"
import { reusePanel } from "@/panels/transforms/reuse-panel"

import {
  keepsPreviousData,
  panelDataKey,
  panelFingerprint,
  sourceScope,
  stateScope,
  variableValuesKey,
  type PanelSource,
} from "./panel-source"

const version: PanelSource = {
  kind: "version",
  dashboardId: "dash",
  version: 3,
}
const build: PanelSource = { kind: "build", requestId: "req", saves: 4 }
const params = { selections: {} }

describe("panelFingerprint", () => {
  it("does not depend on key order", () => {
    expect(
      panelFingerprint({ id: "p", type: "stat", display: { a: 1, b: 2 } })
    ).toBe(panelFingerprint({ display: { b: 2, a: 1 }, type: "stat", id: "p" }))
  })

  it.each([
    ["a changed title", { id: "p", title: "B" }],
    [
      "a changed nested field",
      { id: "p", title: "A", display: { unit: "ms" } },
    ],
    ["a removed field", { id: "p" }],
  ])("changes with %s", (_name, changed) => {
    expect(panelFingerprint(changed)).not.toBe(
      panelFingerprint({ id: "p", title: "A", display: { unit: "s" } })
    )
  })

  it("keeps array order significant", () => {
    expect(panelFingerprint({ q: [1, 2] })).not.toBe(
      panelFingerprint({ q: [2, 1] })
    )
  })

  it("changes for a reusing panel when the panel it reuses changes its query", () => {
    const grid = { x: 0, y: 0, w: 6, h: 4 }
    const reusing = {
      id: "b",
      type: "table",
      gridPos: grid,
      queries: [{ ref: "R", source: { panel: "a", withTransforms: false } }],
    } as DataPanel
    const sourceWith = (query: string) =>
      ({
        id: "a",
        type: "timeseries",
        gridPos: grid,
        datasource: "prom",
        queries: [{ ref: "A", query }],
      }) as DataPanel
    expect(panelFingerprint(reusePanel(reusing, sourceWith("up")))).not.toBe(
      panelFingerprint(reusePanel(reusing, sourceWith("rate(up[5m])")))
    )
  })
})

describe("panel data keys", () => {
  it("keeps the version keys unchanged", () => {
    expect(panelDataKey(version, "p1", "ignored", params)).toEqual([
      "dashboards",
      "dash",
      "versions",
      3,
      "panels",
      "p1",
      params,
    ])
    expect(variableValuesKey(version, "host", "ignored", params)).toEqual([
      "dashboards",
      "dash",
      "versions",
      3,
      "variables",
      "host",
      params,
    ])
  })

  it("keys build data by request and panel fingerprint, not by save count", () => {
    const first = panelDataKey(build, "p1", "fp", params)
    const later = panelDataKey({ ...build, saves: 9 }, "p1", "fp", params)
    expect(first).toEqual(later)
    expect(first).toEqual([
      "observe-build",
      "req",
      "panels",
      "p1",
      "fp",
      params,
    ])
    expect(panelDataKey(build, "p1", "other", params)).not.toEqual(first)
    expect(variableValuesKey(build, "host", "fp", params)).toEqual([
      "observe-build",
      "req",
      "variables",
      "host",
      "fp",
      params,
    ])
  })
})

describe("keepsPreviousData", () => {
  it("keeps data across a refetch of the same version only", () => {
    const key = panelDataKey(version, "p1", "", params)
    expect(keepsPreviousData(version, "", key)).toBe(true)
    expect(keepsPreviousData({ ...version, version: 4 }, "", key)).toBe(false)
    expect(keepsPreviousData({ ...version, dashboardId: "x" }, "", key)).toBe(
      false
    )
  })

  it("keeps build data only while the panel is unchanged", () => {
    const key = panelDataKey(build, "p1", "fp", params)
    expect(keepsPreviousData(build, "fp", key)).toBe(true)
    expect(keepsPreviousData(build, "changed", key)).toBe(false)
    expect(keepsPreviousData({ ...build, requestId: "x" }, "fp", key)).toBe(
      false
    )
  })
})

describe("scopes", () => {
  it("scopes a version by dashboard and version and a build by request", () => {
    expect(sourceScope(version)).toBe("dash:3")
    expect(stateScope(version)).toBe("dash")
    expect(sourceScope(build)).toBe("build:req")
    expect(stateScope(build)).toBe("build:req")
    expect(sourceScope({ ...build, saves: 5 })).toBe(sourceScope(build))
  })
})
