import { describe, expect, it } from "vitest"

import { inRect } from "../anchors"

import { hides, hitTest, locate, type PieAnchorLayout } from "./anchors"

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

describe("piechart locate", () => {
  it("finds the slice a hit test named", () => {
    const point = { x: 170, y: 100 }
    const anchor = hitTest(layout, point)
    expect(anchor).not.toBeNull()
    const rect = locate(layout, anchor!)
    expect(rect).not.toBeNull()
    expect(inRect(rect!, point)).toBe(true)
  })

  it("bounds a quarter of a ring by its corners and the arc's extremes", () => {
    const rect = locate(layout, {
      kind: "slice",
      pos: { x: 0, y: 0 },
      category: "500",
    })
    expect(rect!.x).toBeCloseTo(20)
    expect(rect!.y).toBeCloseTo(20)
    expect(rect!.width).toBeCloseTo(80)
    expect(rect!.height).toBeCloseTo(80)
  })

  it("bounds a whole pie by its circle", () => {
    const pie = {
      ...layout,
      inner: 0,
      slices: [
        { name: "all", text: "9", startAngle: 0, endAngle: Math.PI * 2 },
      ],
    }
    const rect = locate(pie, {
      kind: "slice",
      pos: { x: 0, y: 0 },
      category: "all",
    })
    expect(rect!.x).toBeCloseTo(20)
    expect(rect!.width).toBeCloseTo(160)
    expect(rect!.height).toBeCloseTo(160)
  })

  it("returns null when the slice is gone", () => {
    const pos = { x: 0, y: 0 }
    expect(locate(layout, { kind: "slice", pos, category: "404" })).toBeNull()
    expect(locate(layout, { kind: "position", pos })).toBeNull()
  })
})

describe("piechart hides", () => {
  const pos = { x: 0, y: 0 }
  const toggled = { ...layout, hidden: ["404"] }

  it("hides a slice the legend has switched off", () => {
    expect(hides(toggled, { kind: "slice", pos, category: "404" })).toBe(true)
  })

  it("keeps a slice that is drawn, one that is gone and a different kind", () => {
    expect(hides(toggled, { kind: "slice", pos, category: "200" })).toBe(false)
    expect(hides(toggled, { kind: "slice", pos, category: "503" })).toBe(false)
    expect(hides(layout, { kind: "slice", pos, category: "404" })).toBe(false)
    expect(hides(toggled, { kind: "position", pos })).toBe(false)
  })
})
