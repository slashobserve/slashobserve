import { describe, expect, it } from "vitest"

import { inRect } from "../anchors"

import {
  hides,
  hitTest,
  locate,
  rangeTest,
  type TimelineAnchorLayout,
} from "./anchors"

const layout: TimelineAnchorLayout = {
  body: { width: 300, height: 100 },
  plot: { x: 100, y: 0, width: 200, height: 60 },
  range: { from: 0, to: 2000 },
  rowHeight: 30,
  rows: [
    {
      name: "checkout",
      segments: [
        { from: 0, to: 1000, text: "up", color: "green", value: 1 },
        { from: 1000, to: 2000, text: "down", color: "red", value: 0 },
      ],
    },
    { name: "payments", segments: [] },
  ],
}

describe("state timeline hitTest", () => {
  it("names the state of the row at the pointer's time", () => {
    expect(hitTest(layout, { x: 100 + 150, y: 10 })).toEqual({
      kind: "point",
      pos: { x: 250 / 300, y: 0.1 },
      series: { name: "checkout" },
      from: new Date(1500).toISOString(),
      shown: "down",
    })
  })

  it("names the series when the row has no state there", () => {
    expect(hitTest(layout, { x: 150, y: 40 })).toMatchObject({
      kind: "series",
      series: { name: "payments" },
    })
  })

  it("returns null outside the plot and below the last row", () => {
    expect(hitTest(layout, { x: 20, y: 10 })).toBeNull()
    expect(hitTest(layout, { x: 150, y: 70 })).toBeNull()
  })
})

describe("state timeline locate", () => {
  it("finds the state a hit test named", () => {
    const point = { x: 100 + 150, y: 10 }
    const anchor = hitTest(layout, point)
    expect(anchor).not.toBeNull()
    const rect = locate(layout, anchor!)
    expect(rect).toEqual({ x: 200, y: 0, width: 100, height: 30 })
    expect(inRect(rect!, point)).toBe(true)
  })

  it("finds the row a series anchor named", () => {
    const anchor = hitTest(layout, { x: 150, y: 40 })
    expect(anchor).toMatchObject({ kind: "series" })
    expect(locate(layout, anchor!)).toEqual({
      x: 100,
      y: 30,
      width: 200,
      height: 30,
    })
  })

  it("returns null when the row or the state at that time is gone", () => {
    const pos = { x: 0, y: 0 }
    const from = new Date(1500).toISOString()
    expect(
      locate(layout, { kind: "point", pos, series: { name: "orders" }, from })
    ).toBeNull()
    expect(
      locate(layout, {
        kind: "point",
        pos,
        series: { name: "payments" },
        from,
      })
    ).toBeNull()
    expect(
      locate(layout, {
        kind: "point",
        pos,
        series: { name: "checkout" },
        from: "not a time",
      })
    ).toBeNull()
    expect(locate(layout, { kind: "position", pos })).toBeNull()
  })
})

describe("state timeline rangeTest", () => {
  const start = { x: 100 + 50, y: 10 }
  const end = { x: 100 + 150, y: 40 }

  it("names the time span of a drag, whichever way it goes", () => {
    const expected = {
      kind: "range",
      pos: { x: 200 / 300, y: 25 / 100 },
      from: new Date(500).toISOString(),
      to: new Date(1500).toISOString(),
    }
    expect(rangeTest(layout, start, end)).toEqual(expected)
    expect(rangeTest(layout, end, start)).toEqual(expected)
  })

  it("is found again as a band across the plot", () => {
    const anchor = rangeTest(layout, start, end)
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual({
      x: 150,
      y: 0,
      width: 100,
      height: 60,
    })
  })

  it("returns null for a drag that leaves the plot", () => {
    expect(rangeTest(layout, start, { x: 20, y: 10 })).toBeNull()
  })

  it("reads a range outside the time range as gone", () => {
    expect(
      locate(layout, {
        kind: "range",
        pos: { x: 0, y: 0 },
        from: new Date(5_000).toISOString(),
        to: new Date(6_000).toISOString(),
      })
    ).toBeNull()
  })
})

describe("state timeline hides", () => {
  const pos = { x: 0, y: 0 }
  const iso = (ms: number) => new Date(ms).toISOString()

  it("hides a point or a range outside the shown time range", () => {
    expect(
      hides(layout, {
        kind: "point",
        pos,
        series: { name: "checkout" },
        from: iso(3000),
      })
    ).toBe(true)
    expect(
      hides(layout, {
        kind: "point",
        pos,
        series: { name: "checkout" },
        from: iso(-1),
      })
    ).toBe(true)
    expect(
      hides(layout, { kind: "range", pos, from: iso(2001), to: iso(3000) })
    ).toBe(true)
    expect(
      hides(layout, { kind: "range", pos, from: iso(-500), to: iso(-1) })
    ).toBe(true)
  })

  it("keeps what the range shows, wholly or partly, and what has no time", () => {
    expect(
      hides(layout, {
        kind: "point",
        pos,
        series: { name: "checkout" },
        from: iso(500),
      })
    ).toBe(false)
    expect(
      hides(layout, { kind: "range", pos, from: iso(1500), to: iso(3000) })
    ).toBe(false)
    expect(
      hides(layout, { kind: "series", pos, series: { name: "gone" } })
    ).toBe(false)
  })
})
