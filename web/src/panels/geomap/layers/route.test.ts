import { describe, expect, it } from "vitest"

import type { Tip } from "../geo-model"
import { arrowOf, routeModel, type RouteRow } from "./route"

const tip: Tip = { label: "x", details: [], color: "c", state: null }

function row(frameIndex: number, index: number, time: number | null): RouteRow {
  return { frameIndex, row: index, lat: index, lon: index, time, tip }
}

describe("routeModel", () => {
  it("orders each frame's points by time", () => {
    const model = routeModel(
      [row(0, 0, 300), row(0, 1, 100), row(0, 2, 200)],
      {},
      undefined
    )
    expect(model.lines).toHaveLength(1)
    expect(model.lines[0].points.map((point) => point.row)).toEqual([1, 2, 0])
  })

  it("keeps the row order when a frame has no time for some row", () => {
    const model = routeModel(
      [row(0, 0, 300), row(0, 1, null), row(0, 2, 200)],
      {},
      undefined
    )
    expect(model.lines[0].points.map((point) => point.row)).toEqual([0, 1, 2])
  })

  it("keeps rows with equal times in their original order", () => {
    const model = routeModel(
      [row(0, 0, 5), row(0, 1, 5), row(0, 2, 5)],
      {},
      undefined
    )
    expect(model.lines[0].points.map((point) => point.row)).toEqual([0, 1, 2])
  })

  it("draws one line per frame in the series palette, or the chosen color", () => {
    const rows = [row(0, 0, 1), row(1, 0, 1), row(1, 1, 2)]
    const palette = routeModel(rows, {}, undefined)
    expect(palette.lines.map((line) => line.points.length)).toEqual([1, 2])
    expect(palette.lines[0].color).toBe("var(--series-1)")
    expect(palette.lines[1].color).toBe("var(--series-2)")
    expect(routeModel(rows, {}, "var(--threshold-ok)").lines[1].color).toBe(
      "var(--threshold-ok)"
    )
  })

  it("defaults the width to 2 and the arrow to none", () => {
    expect(routeModel([], {}, undefined)).toMatchObject({
      lineWidth: 2,
      arrow: "none",
    })
    expect(
      routeModel([], { lineWidth: 5, arrow: "both" }, undefined)
    ).toMatchObject({ lineWidth: 5, arrow: "both" })
  })
})

describe("arrowOf", () => {
  it("accepts the four arrow modes and ignores anything else", () => {
    for (const arrow of ["none", "forward", "reverse", "both"]) {
      expect(arrowOf(arrow)).toBe(arrow)
    }
    expect(arrowOf("sideways")).toBe("none")
    expect(arrowOf(undefined)).toBe("none")
  })
})
