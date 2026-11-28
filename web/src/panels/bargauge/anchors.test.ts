import { describe, expect, it } from "vitest"

import type { ValueAnchorLayout } from "../value-anchors"

import { hitTest, locate } from "./anchors"

function nth<T>(items: ArrayLike<T>, index: number): T {
  const item = items[index]
  expect(item).toBeDefined()
  return item as T
}

const layout: ValueAnchorLayout = {
  body: { width: 200, height: 100 },
  targets: [
    {
      rect: { x: 0, y: 0, width: 200, height: 40 },
      name: "cpu",
      labels: { instance: "a" },
      text: "62%",
    },
    {
      rect: { x: 0, y: 50, width: 200, height: 40 },
      name: "memory",
      labels: undefined,
      text: "41%",
    },
  ],
}

describe("bar gauge hitTest", () => {
  it("names the value under the pointer", () => {
    expect(hitTest(layout, { x: 50, y: 20 })).toEqual({
      kind: "value",
      pos: { x: 0.25, y: 0.2 },
      series: { instance: "a" },
      shown: "62%",
    })
  })

  it("names an unlabelled value by its name", () => {
    expect(hitTest(layout, { x: 50, y: 60 })).toMatchObject({
      series: { name: "memory" },
      shown: "41%",
    })
  })

  it("returns null between values", () => {
    expect(hitTest(layout, { x: 50, y: 45 })).toBeNull()
  })
})

describe("bar gauge locate", () => {
  it("finds the value a hit test named", () => {
    const anchor = hitTest(layout, { x: 50, y: 60 })
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual(nth(layout.targets, 1).rect)
  })

  it("returns null when the value is gone", () => {
    expect(
      locate(layout, {
        kind: "value",
        pos: { x: 0, y: 0 },
        series: { name: "disk" },
      })
    ).toBeNull()
  })
})
