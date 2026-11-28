import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { HeatmapAnchorLayout } from "./anchors"
import HeatmapPanel from "./heatmap-panel"

function bucket(le: string, values: number[]): PreparedFrame {
  return {
    ref: "A",
    labels: { le },
    fields: [
      { name: "Time", type: "time", values: [1_000, 61_000, 121_000] },
      { name: "Value", type: "number", values },
    ],
  } as PreparedFrame
}

const frames = [
  bucket("0.1", [1, 2, 3]),
  bucket("0.5", [2, 4, 6]),
  bucket("+Inf", [3, 6, 9]),
]

function panel() {
  const target = panelOf("heatmap")
  target.queries = [{ query: "x", format: "heatmap" }] as typeof target.queries
  return target
}

describe("heatmap anchors", () => {
  it("publishes the plot it drew with its rows and time steps", () => {
    const { store } = renderPublishing(HeatmapPanel, panel(), frames)
    const published = store.current()
    expect(published?.type).toBe("heatmap")
    const layout = published?.layout as HeatmapAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(layout.model.rows.map(({ label }) => label)).toEqual([
      "0.1",
      "0.5",
      "+Inf",
    ])
    expect(layout.model.xs).toHaveLength(3)
    expect(layout.plot.width).toBeGreaterThan(0)
    expect(layout.plot.height).toBeGreaterThan(0)
  })
})
