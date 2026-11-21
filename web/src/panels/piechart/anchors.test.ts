import { describe, expect, it } from "vitest"

import { hitTest, type PieAnchorLayout } from "./anchors"

const layout: PieAnchorLayout = {
  body: { width: 200, height: 200 },
  center: { x: 100, y: 100 },
  outer: 80,
  inner: 40,
  slices: [
    { name: "200", text: "75", startAngle: 0, endAngle: Math.PI * 1.5 },
    {
      name: "500",
      text: "25",
      startAngle: Math.PI * 1.5,
      endAngle: Math.PI * 2,
    },
  ],
}

describe("piechart hitTest", () => {
  it("names the slice under the pointer, clockwise from twelve o'clock", () => {
    expect(hitTest(layout, { x: 170, y: 100 })).toEqual({
      kind: "slice",
      pos: { x: 0.85, y: 0.5 },
      category: "200",
      shown: "75",
    })
    expect(hitTest(layout, { x: 40, y: 60 })).toMatchObject({
      category: "500",
      shown: "25",
    })
  })

  it("returns null in the donut hole and outside the ring", () => {
    expect(hitTest(layout, { x: 100, y: 100 })).toBeNull()
    expect(hitTest(layout, { x: 190, y: 100 })).toBeNull()
  })

  it("names the only slice of a full ring", () => {
    const full = {
      ...layout,
      slices: [
        { name: "all", text: "9", startAngle: 0, endAngle: Math.PI * 2 },
      ],
    }
    expect(hitTest(full, { x: 100, y: 30 })).toMatchObject({ category: "all" })
  })
})
