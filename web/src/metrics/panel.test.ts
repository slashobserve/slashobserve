import { describe, expect, it } from "vitest"

import { appendPanel } from "@/dashboards/append-panel"

import { metricsPanelFor } from "./panel"
import { must } from "@/test/must"

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

describe("appending a metrics panel", () => {
  it("places it under the last panel with a free id", () => {
    const spec = {
      specVersion: "0",
      title: "Shop",
      time: { from: "now-6h", to: "now" },
      panels: [
        metricsPanelFor({ ...base, id: "metrics", mode: "graph", y: 0 }),
      ],
    } as Parameters<typeof appendPanel>[0]
    const next = appendPanel(spec, "metrics", (slot) =>
      metricsPanelFor({ ...base, ...slot, mode: "table" })
    )
    expect(next.panels.map((panel) => panel.id)).toEqual([
      "metrics",
      "metrics-2",
    ])
    expect(must(next.panels[1]).gridPos.y).toBe(10)
    expect(must(next.panels[1]).type).toBe("table")
  })
})
