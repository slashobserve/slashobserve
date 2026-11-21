import { describe, expect, it } from "vitest"

import { inRect } from "../anchors"

import {
  hides,
  hitTest,
  locate,
  rangeTest,
  type HeatmapAnchorLayout,
} from "./anchors"

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

describe("heatmap locate", () => {
  it("finds the cell a hit test named", () => {
    const point = { x: 40 + 60, y: 10 + 75 }
    const anchor = hitTest(layout, point)
    expect(anchor).not.toBeNull()
    const rect = locate(layout, anchor!)
    expect(rect).toEqual({ x: 40 + 50, y: 10 + 50, width: 50, height: 50 })
    expect(inRect(rect!, point)).toBe(true)
  })

  it("returns null when the bucket or the time step is gone", () => {
    const at = new Date(60_000).toISOString()
    const pos = { x: 0, y: 0 }
    expect(
      locate(layout, { kind: "heat_cell", pos, from: at, category: "1s" })
    ).toBeNull()
    expect(
      locate(layout, {
        kind: "heat_cell",
        pos,
        from: new Date(61_000).toISOString(),
        category: "10ms",
      })
    ).toBeNull()
    expect(locate(layout, { kind: "position", pos })).toBeNull()
  })
})

describe("heatmap rangeTest", () => {
  const start = { x: 40 + 50, y: 10 + 20 }
  const end = { x: 40 + 150, y: 10 + 60 }

  it("names the time span of a drag, whichever way it goes", () => {
    const expected = {
      kind: "range",
      pos: { x: 140 / 240, y: 50 / 140 },
      from: new Date(60_000).toISOString(),
      to: new Date(180_000).toISOString(),
    }
    expect(rangeTest(layout, start, end)).toEqual(expected)
    expect(rangeTest(layout, end, start)).toEqual(expected)
  })

  it("is found again as a band across the plot", () => {
    const anchor = rangeTest(layout, start, end)
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual({
      x: 40 + 50,
      y: 10,
      width: 100,
      height: 100,
    })
  })

  it("reads the columns of a gappy heatmap by index, not by an even time step", () => {
    const gappy = {
      ...layout,
      model: { ...layout.model, xs: [0, 60_000, 120_000, 600_000] },
    }
    const anchor = rangeTest(gappy, start, end)
    expect(anchor).toMatchObject({
      from: new Date(60_000).toISOString(),
      to: new Date(600_000).toISOString(),
    })
    expect(locate(gappy, anchor!)).toEqual({
      x: 40 + 50,
      y: 10,
      width: 100,
      height: 100,
    })
  })

  it("places a drag inside a wide column partway along that column's time span", () => {
    const gappy = {
      ...layout,
      model: { ...layout.model, xs: [0, 60_000, 120_000, 600_000] },
    }
    const anchor = rangeTest(
      gappy,
      { x: 40 + 25, y: 20 },
      { x: 40 + 175, y: 40 }
    )
    expect(anchor).toMatchObject({
      from: new Date(30_000).toISOString(),
      to: new Date(600_000 + 0.5 * 480_000).toISOString(),
    })
  })

  it("returns null for a drag that leaves the plot or a plot with no time span", () => {
    expect(rangeTest(layout, start, { x: 10, y: 50 })).toBeNull()
    const single = { ...layout, model: { ...layout.model, xs: [0] } }
    expect(rangeTest(single, start, end)).toBeNull()
  })

  it("reads a range outside the time steps as gone", () => {
    const pos = { x: 0, y: 0 }
    expect(
      locate(layout, {
        kind: "range",
        pos,
        from: new Date(300_000).toISOString(),
        to: new Date(400_000).toISOString(),
      })
    ).toBeNull()
  })
})

describe("heatmap hides", () => {
  const pos = { x: 0, y: 0 }
  const iso = (ms: number) => new Date(ms).toISOString()

  it("hides a cell or a range outside the shown time range", () => {
    expect(
      hides(layout, {
        kind: "heat_cell",
        pos,
        from: iso(300_000),
        category: "10ms",
      })
    ).toBe(true)
    expect(
      hides(layout, {
        kind: "heat_cell",
        pos,
        from: iso(-60_000),
        category: "10ms",
      })
    ).toBe(true)
    expect(
      hides(layout, {
        kind: "range",
        pos,
        from: iso(300_000),
        to: iso(400_000),
      })
    ).toBe(true)
    expect(
      hides(layout, { kind: "range", pos, from: iso(-9), to: iso(-1) })
    ).toBe(true)
  })

  it("keeps a cell in range with a missing bucket, and a range partly shown", () => {
    expect(
      hides(layout, {
        kind: "heat_cell",
        pos,
        from: iso(60_000),
        category: "gone",
      })
    ).toBe(false)
    expect(
      hides(layout, {
        kind: "range",
        pos,
        from: iso(200_000),
        to: iso(400_000),
      })
    ).toBe(false)
  })
})
