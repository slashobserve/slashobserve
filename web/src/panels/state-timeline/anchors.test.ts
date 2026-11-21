import { describe, expect, it } from "vitest"

import { hitTest, type TimelineAnchorLayout } from "./anchors"

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
