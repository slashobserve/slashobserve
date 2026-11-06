import { describe, expect, it } from "vitest"

import type { Dashboard, Panel } from "@/spec"

import { mixesDatasources, panelDatasourceUids } from "./panel-datasources"

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
    expect(
      panelDatasourceUids(dataPanel("prom", [{ query: "up" }]), {})
    ).toEqual(["prom"])
  })

  it("adds per-query datasources after the panel's", () => {
    expect(
      panelDatasourceUids(
        dataPanel("prom", [
          { query: "up" },
          { query: "select 1", datasource: "ch" },
          { query: "up", datasource: "other" },
        ]),
        {}
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
        ]),
        {}
      )
    ).toEqual(["prom", "ch"])
    expect(panelDatasourceUids(dataPanel("", [{ query: "up" }]), {})).toEqual(
      []
    )
  })

  it("returns nothing for a row panel", () => {
    expect(
      panelDatasourceUids({ id: "r1", type: "row", gridPos, title: "Row" }, {})
    ).toEqual([])
  })

  it("names the selected datasource for a ${name} reference", () => {
    expect(
      panelDatasourceUids(dataPanel("${ds}", [{ query: "up" }]), {
        ds: ["prom"],
      })
    ).toEqual(["prom"])
  })

  it("merges a reference with a query override on the same datasource", () => {
    expect(
      panelDatasourceUids(
        dataPanel("${ds}", [{ query: "up", datasource: "prom" }]),
        { ds: ["prom"] }
      )
    ).toEqual(["prom"])
  })

  it("keeps an unresolved reference as written", () => {
    expect(
      panelDatasourceUids(dataPanel("${ds}", [{ query: "up" }]), {})
    ).toEqual(["${ds}"])
  })
})

describe("mixesDatasources", () => {
  function dashboard(panels: Panel[]): Dashboard {
    return { title: "D", panels } as unknown as Dashboard
  }
  const text = {
    id: "t1",
    type: "text",
    gridPos,
    content: "hi",
  } as unknown as Panel

  it.each<{
    name: string
    panels: Panel[]
    selections: Record<string, string[]>
    mixed: boolean
  }>([
    {
      name: "is false when every panel reads one datasource",
      panels: [
        dataPanel("prom", [{ query: "up" }]),
        dataPanel("prom", [{ query: "up", datasource: "prom" }]),
        text,
      ],
      selections: {},
      mixed: false,
    },
    {
      name: "is false for a dashboard without data panels",
      panels: [text],
      selections: {},
      mixed: false,
    },
    {
      name: "is true when two panels read different datasources",
      panels: [
        dataPanel("prom", [{ query: "up" }]),
        dataPanel("ch", [{ query: "select 1" }]),
      ],
      selections: {},
      mixed: true,
    },
    {
      name: "is true when one panel mixes datasources across its queries",
      panels: [
        dataPanel("prom", [{ query: "up" }, { query: "x", datasource: "ch" }]),
      ],
      selections: {},
      mixed: true,
    },
    {
      name: "counts the panels inside a row",
      panels: [
        dataPanel("prom", [{ query: "up" }]),
        {
          id: "r1",
          type: "row",
          gridPos,
          title: "Row",
          panels: [dataPanel("ch", [{ query: "select 1" }])],
        } as unknown as Panel,
      ],
      selections: {},
      mixed: true,
    },
    {
      name: "resolves a datasource variable to its selection",
      panels: [
        dataPanel("${ds}", [{ query: "up" }]),
        dataPanel("prom", [{ query: "up" }]),
      ],
      selections: { ds: ["prom"] },
      mixed: false,
    },
  ])("$name", ({ panels, selections, mixed }) => {
    expect(mixesDatasources(dashboard(panels), selections)).toBe(mixed)
  })
})
