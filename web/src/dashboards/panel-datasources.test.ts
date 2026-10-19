import { describe, expect, it } from "vitest"

import type { Panel } from "@/spec"

import { panelDatasourceUids } from "./panel-datasources"

const gridPos = { x: 0, y: 0, w: 12, h: 6 }

function dataPanel(
  datasource: string,
  queries: { query: string; datasource?: string }[]
): Panel {
  return {
    id: "p1",
    type: "timeseries",
    title: "Panel",
    gridPos,
    datasource,
    queries,
  }
}

describe("panelDatasourceUids", () => {
  it("returns the panel's own datasource", () => {
    expect(panelDatasourceUids(dataPanel("prom", [{ query: "up" }]))).toEqual([
      "prom",
    ])
  })

  it("adds per-query datasources after the panel's", () => {
    expect(
      panelDatasourceUids(
        dataPanel("prom", [
          { query: "up" },
          { query: "select 1", datasource: "ch" },
          { query: "up", datasource: "other" },
        ])
      )
    ).toEqual(["prom", "ch", "other"])
  })

  it("drops duplicates and empty uids", () => {
    expect(
      panelDatasourceUids(
        dataPanel("prom", [
          { query: "up", datasource: "prom" },
          { query: "up", datasource: "ch" },
          { query: "up", datasource: "ch" },
          { query: "up", datasource: "" },
        ])
      )
    ).toEqual(["prom", "ch"])
    expect(panelDatasourceUids(dataPanel("", [{ query: "up" }]))).toEqual([])
  })

  it("returns nothing for a row panel", () => {
    expect(
      panelDatasourceUids({ id: "r1", type: "row", gridPos, title: "Row" })
    ).toEqual([])
  })
})
