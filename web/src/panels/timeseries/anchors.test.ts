import { describe, expect, it } from "vitest"

import {
  hides,
  hitTest,
  locate,
  pointRadius,
  rangeTest,
  type TimeseriesAnchorLayout,
} from "./anchors"

const layout: TimeseriesAnchorLayout = {
  body: { width: 400, height: 200 },
  plot: { x: 40, y: 10, width: 300, height: 150 },
  xMin: 1000,
  xMax: 1300,
  xs: [1000, 1100, 1200, 1300],
  series: [
    {
      name: "checkout",
      labels: { service: "checkout" },
      ys: [100, 50, 50, null],
      values: [1, 4, 4, null],
      format: (value) => `${value} req/s`,
    },
    {
      name: "payments",
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

const iso = (seconds: number) => new Date(seconds * 1000).toISOString()

describe("timeseries locate", () => {
  it("finds the sample a point anchor names", () => {
    expect(
      locate(layout, {
        kind: "point",
        pos: { x: 0, y: 0 },
        from: iso(1100),
        series: { service: "checkout" },
      })
    ).toEqual({
      x: 140 - pointRadius,
      y: 60 - pointRadius,
      width: pointRadius * 2,
      height: pointRadius * 2,
    })
  })

  it("matches a series by the labels the anchor kept", () => {
    const wide = {
      ...layout,
      series: layout.series.map((entry) => ({
        ...entry,
        labels: { ...entry.labels, pod: "a" },
      })),
    }
    expect(
      locate(wide, {
        kind: "point",
        pos: { x: 0, y: 0 },
        from: iso(1200),
        series: { service: "payments" },
      })
    ).toMatchObject({ x: 240 - pointRadius, y: 30 - pointRadius })
  })

  it.each([
    ["the series is gone", { service: "search" }, 1100],
    ["the time left the window", { service: "checkout" }, 5000],
    ["the sample has no value", { service: "checkout" }, 1300],
  ])("returns null when %s", (_why, series, seconds) => {
    expect(
      locate(layout, {
        kind: "point",
        pos: { x: 0, y: 0 },
        from: iso(seconds),
        series,
      })
    ).toBeNull()
  })

  it("draws a band for a time and a range", () => {
    expect(
      locate(layout, { kind: "time", pos: { x: 0, y: 0 }, from: iso(1100) })
    ).toEqual({ x: 139, y: 10, width: 2, height: 150 })
    expect(
      locate(layout, {
        kind: "range",
        pos: { x: 0, y: 0 },
        from: iso(1100),
        to: iso(1200),
      })
    ).toEqual({ x: 139, y: 10, width: 102, height: 150 })
    expect(
      locate(layout, {
        kind: "range",
        pos: { x: 0, y: 0 },
        from: iso(5000),
        to: iso(6000),
      })
    ).toBeNull()
  })

  it("finds the series nearest the stored spot", () => {
    expect(
      locate(layout, {
        kind: "series",
        pos: { x: 240 / 400, y: 0 },
        series: { service: "payments" },
      })
    ).toMatchObject({ x: 240 - pointRadius, y: 30 - pointRadius })
  })
})

describe("timeseries hides", () => {
  const iso = (seconds: number) => new Date(seconds * 1000).toISOString()
  const pos = { x: 0, y: 0 }
  const legendHid = { ...layout, hidden: [{ service: "search" }] }

  it("hides a moment, a point or a whole range outside the shown time", () => {
    expect(hides(layout, { kind: "time", pos, from: iso(900) })).toBe(true)
    expect(
      hides(layout, {
        kind: "point",
        pos,
        from: iso(1400),
        series: { service: "checkout" },
      })
    ).toBe(true)
    expect(
      hides(layout, { kind: "range", pos, from: iso(1301), to: iso(1500) })
    ).toBe(true)
  })

  it("shows a moment in view and a range that overlaps it", () => {
    expect(hides(layout, { kind: "time", pos, from: iso(1100) })).toBe(false)
    expect(
      hides(layout, { kind: "range", pos, from: iso(900), to: iso(1050) })
    ).toBe(false)
  })

  it("hides a series the legend hid, and not a series that is missing", () => {
    expect(
      hides(legendHid, { kind: "series", pos, series: { service: "search" } })
    ).toBe(true)
    expect(
      hides(legendHid, {
        kind: "point",
        pos,
        from: iso(1100),
        series: { service: "search" },
      })
    ).toBe(true)
    expect(
      hides(legendHid, { kind: "series", pos, series: { service: "gone" } })
    ).toBe(false)
  })
})
