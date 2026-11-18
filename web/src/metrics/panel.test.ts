import { describe, expect, it } from "vitest"

import { metricsPanelFor } from "./panel"

const base = {
  id: "p1",
  title: "Requests",
  datasource: "prom",
  query: "up",
  y: 4,
}

describe("metricsPanelFor", () => {
  it("builds a time series with a legend table for a graph", () => {
    expect(metricsPanelFor({ ...base, mode: "graph" })).toEqual({
      id: "p1",
      title: "Requests",
      datasource: "prom",
      gridPos: { x: 0, y: 4, w: 24, h: 10 },
      type: "timeseries",
      queries: [{ query: "up" }],
      display: {
        legend: "bottom",
        legendMode: "table",
        legendValues: ["mean", "max", "last"],
      },
    })
  })

  it("builds a table of an instant query for the table mode", () => {
    expect(metricsPanelFor({ ...base, mode: "table" })).toEqual({
      id: "p1",
      title: "Requests",
      datasource: "prom",
      gridPos: { x: 0, y: 4, w: 24, h: 10 },
      type: "table",
      queries: [{ query: "up", instant: true, format: "table" }],
    })
  })
})
