import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"

import { reusePanel } from "./reuse-panel"

const grid = { x: 0, y: 0, w: 6, h: 4 }

const source = {
  id: "src",
  type: "timeseries",
  gridPos: grid,
  datasource: "prom",
  maxDataPoints: 300,
  queries: [{ ref: "S", query: "up" }],
  transform: [{ type: "merge" }],
} as DataPanel

function dependent(withTransforms: boolean, own?: DataPanel["transform"]) {
  return {
    id: "dep",
    type: "table",
    gridPos: grid,
    queries: [{ ref: "R", source: { panel: "src", withTransforms } }],
    transform: own,
    overrides: [{ match: { query: "S" } }],
  } as DataPanel
}

describe("reusePanel", () => {
  it("takes the source's datasource, queries and settings", () => {
    const got = reusePanel(dependent(false), source)
    expect(got.datasource).toBe("prom")
    expect(got.queries).toBe(source.queries)
    expect(got.maxDataPoints).toBe(300)
    expect(got.id).toBe("dep")
    expect(got.type).toBe("table")
    expect(got.overrides).toEqual(dependent(false).overrides)
  })

  it("leaves the source's transforms out unless withTransforms is set", () => {
    expect(reusePanel(dependent(false), source).transform).toBeUndefined()
    expect(
      reusePanel(dependent(false, [{ type: "seriesToRows" }]), source).transform
    ).toEqual([{ type: "seriesToRows" }])
  })

  it("runs the source's transforms before the panel's own", () => {
    const got = reusePanel(dependent(true, [{ type: "seriesToRows" }]), source)
    expect(got.transform).toEqual([{ type: "merge" }, { type: "seriesToRows" }])
  })

  it("returns a panel without a dashboard query unchanged", () => {
    expect(reusePanel(source, source)).toBe(source)
  })

  it("returns the panel unchanged when the source is missing", () => {
    const panel = dependent(true)
    expect(reusePanel(panel, undefined)).toBe(panel)
  })
})
