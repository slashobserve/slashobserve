import { describe, expect, it } from "vitest"

import { hitTest, type HeatmapAnchorLayout } from "./anchors"

const layout: HeatmapAnchorLayout = {
  body: { width: 240, height: 140 },
  origin: { x: 40, y: 10 },
  plot: { width: 200, height: 100 },
  model: {
    xs: [0, 60_000, 120_000, 180_000],
    rows: [
      { label: "10ms", range: "0-10ms" },
      { label: "50ms", range: "10-50ms" },
    ],
    cells: [
      [1, 2, 3, null],
      [4, 5, 6, 7],
    ],
  },
  format: (value) => `${value}`,
}

describe("heatmap hitTest", () => {
  it("names the cell under the pointer, lowest bucket at the bottom", () => {
    expect(hitTest(layout, { x: 40 + 60, y: 10 + 75 })).toEqual({
      kind: "heat_cell",
      pos: { x: 100 / 240, y: 85 / 140 },
      from: new Date(60_000).toISOString(),
      category: "10ms",
      shown: "2",
    })
    expect(hitTest(layout, { x: 40 + 60, y: 10 + 25 })).toMatchObject({
      category: "50ms",
      shown: "5",
    })
  })

  it("leaves shown out of an empty cell", () => {
    const anchor = hitTest(layout, { x: 40 + 190, y: 10 + 75 })
    expect(anchor).toMatchObject({ kind: "heat_cell", category: "10ms" })
    expect(anchor).not.toHaveProperty("shown")
  })

  it("returns null outside the plot and for an empty model", () => {
    expect(hitTest(layout, { x: 10, y: 50 })).toBeNull()
    const empty = { ...layout, model: { xs: [], rows: [], cells: [] } }
    expect(hitTest(empty, { x: 60, y: 50 })).toBeNull()
  })
})
