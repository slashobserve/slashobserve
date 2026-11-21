import { describe, expect, it } from "vitest"

import type { ValueAnchorLayout } from "../value-anchors"

import { hitTest, locate } from "./anchors"

const layout: ValueAnchorLayout = {
  body: { width: 300, height: 100 },
  targets: [
    {
      rect: { x: 0, y: 0, width: 146, height: 100 },
      name: "checkout",
      labels: { service: "checkout" },
      text: "98.2%",
    },
    {
      rect: { x: 154, y: 0, width: 146, height: 100 },
      name: "Availability",
      labels: undefined,
      text: "99.9%",
    },
  ],
}

describe("stat hitTest", () => {
  it("names the value of the tile under the pointer", () => {
    expect(hitTest(layout, { x: 30, y: 50 })).toEqual({
      kind: "value",
      pos: { x: 0.1, y: 0.5 },
      series: { service: "checkout" },
      shown: "98.2%",
    })
  })

  it("names an unlabelled tile by its name", () => {
    expect(hitTest(layout, { x: 200, y: 50 })).toMatchObject({
      series: { name: "Availability" },
      shown: "99.9%",
    })
  })

  it("returns null in the gap between tiles", () => {
    expect(hitTest(layout, { x: 150, y: 50 })).toBeNull()
  })
})

describe("stat locate", () => {
  it("finds the tile a value anchor names", () => {
    expect(
      locate(layout, {
        kind: "value",
        pos: { x: 0, y: 0 },
        series: { name: "Availability" },
      })
    ).toEqual({ x: 154, y: 0, width: 146, height: 100 })
  })

  it("returns null when the tile is gone", () => {
    expect(
      locate(layout, {
        kind: "value",
        pos: { x: 0, y: 0 },
        series: { service: "payments" },
      })
    ).toBeNull()
  })
})
