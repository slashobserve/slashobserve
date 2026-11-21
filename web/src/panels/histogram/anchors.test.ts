import { describe, expect, it } from "vitest"

import { hitTest, type HistogramAnchorLayout } from "./anchors"

const layout: HistogramAnchorLayout = {
  body: { width: 240, height: 140 },
  origin: { x: 40, y: 10 },
  plot: { width: 200, height: 100 },
  edges: [0, 10, 20, 30, 40],
  format: (value) => `${value} ms`,
}

describe("histogram hitTest", () => {
  it("names the bucket under the pointer", () => {
    expect(hitTest(layout, { x: 40 + 120, y: 50 })).toEqual({
      kind: "bar",
      pos: { x: 160 / 240, y: 50 / 140 },
      category: "20 ms to 30 ms",
    })
  })

  it("keeps the last bucket on the right edge", () => {
    expect(hitTest(layout, { x: 240, y: 50 })).toMatchObject({
      category: "30 ms to 40 ms",
    })
  })

  it("returns null outside the plot and without buckets", () => {
    expect(hitTest(layout, { x: 10, y: 50 })).toBeNull()
    expect(hitTest({ ...layout, edges: [0] }, { x: 100, y: 50 })).toBeNull()
  })
})
