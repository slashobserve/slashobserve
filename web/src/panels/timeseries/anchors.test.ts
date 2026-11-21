import { describe, expect, it } from "vitest"

import { hitTest, rangeTest, type TimeseriesAnchorLayout } from "./anchors"

const layout: TimeseriesAnchorLayout = {
  body: { width: 400, height: 200 },
  plot: { x: 40, y: 10, width: 300, height: 150 },
  xMin: 1000,
  xMax: 1300,
  xs: [1000, 1100, 1200, 1300],
  series: [
    {
      labels: { service: "checkout" },
      ys: [100, 50, 50, null],
      values: [1, 4, 4, null],
      format: (value) => `${value} req/s`,
    },
    {
      labels: { service: "payments" },
      ys: [140, 140, 20, 20],
      values: [0, 0, 9, 9],
      format: (value) => `${value} req/s`,
    },
  ],
}

function at(plotX: number, plotY: number) {
  return { x: layout.plot.x + plotX, y: layout.plot.y + plotY }
}

describe("timeseries hitTest", () => {
  it("returns null outside the plot area", () => {
    expect(hitTest(layout, { x: 10, y: 50 })).toBeNull()
    expect(hitTest(layout, at(100, 200))).toBeNull()
  })

  it("points at the sample under the pointer", () => {
    expect(hitTest(layout, at(101, 52))).toEqual({
      kind: "point",
      pos: { x: 141 / 400, y: 62 / 200 },
      from: new Date(1100 * 1000).toISOString(),
      series: { service: "checkout" },
      shown: "4 req/s",
    })
  })

  it("picks the nearer series when two are in reach", () => {
    const anchor = hitTest(layout, at(200, 25))
    expect(anchor).toMatchObject({
      kind: "point",
      series: { service: "payments" },
      shown: "9 req/s",
    })
  })

  it("names the series when the pointer is on the line between samples", () => {
    const anchor = hitTest(layout, at(150, 50))
    expect(anchor).toMatchObject({
      kind: "series",
      series: { service: "checkout" },
    })
    expect(anchor).not.toHaveProperty("from")
  })

  it("falls back to the moment when no series is near", () => {
    expect(hitTest(layout, at(100, 90))).toEqual({
      kind: "time",
      pos: { x: 140 / 400, y: 100 / 200 },
      from: new Date(1100 * 1000).toISOString(),
    })
  })

  it("skips a series with no value at the nearest sample", () => {
    const anchor = hitTest(layout, at(300, 100))
    expect(anchor?.kind).toBe("time")
  })

  it("uses the pointer's time when there are no samples", () => {
    const empty = { ...layout, xs: [], series: [] }
    expect(hitTest(empty, at(150, 50))).toMatchObject({
      kind: "time",
      from: new Date(1150 * 1000).toISOString(),
    })
  })
})

describe("timeseries rangeTest", () => {
  it("orders the ends of a drag", () => {
    expect(rangeTest(layout, at(200, 40), at(100, 60))).toMatchObject({
      kind: "range",
      from: new Date(1100 * 1000).toISOString(),
      to: new Date(1200 * 1000).toISOString(),
    })
  })

  it("needs both ends in the plot", () => {
    expect(rangeTest(layout, at(100, 40), { x: 0, y: 0 })).toBeNull()
  })
})
