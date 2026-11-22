import { describe, expect, it } from "vitest"

import { hitTest, locate, type CanvasAnchorLayout } from "./anchors"

const layout: CanvasAnchorLayout = {
  body: { width: 200, height: 100 },
  elements: [
    { id: "background", rect: { x: 0, y: 0, width: 200, height: 100 } },
    { id: "gauge", rect: { x: 20, y: 20, width: 60, height: 40 } },
  ],
}

describe("canvas hitTest", () => {
  it("names the topmost element under the pointer", () => {
    expect(hitTest(layout, { x: 30, y: 30 })).toEqual({
      kind: "element",
      pos: { x: 0.15, y: 0.3 },
      id: "gauge",
    })
  })

  it("names the element underneath where nothing covers it", () => {
    expect(hitTest(layout, { x: 150, y: 80 })).toMatchObject({
      id: "background",
    })
  })

  it("returns null outside every element", () => {
    expect(hitTest(layout, { x: 250, y: 80 })).toBeNull()
  })
})

describe("canvas locate", () => {
  it("finds the element a hit test named", () => {
    const anchor = hitTest(layout, { x: 30, y: 30 })
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual(layout.elements[1].rect)
  })

  it("takes the topmost of two drawings of one element", () => {
    const shaped: CanvasAnchorLayout = {
      ...layout,
      elements: [
        ...layout.elements,
        { id: "gauge", rect: { x: 25, y: 25, width: 20, height: 10 } },
      ],
    }
    expect(
      locate(shaped, { kind: "element", pos: { x: 0, y: 0 }, id: "gauge" })
    ).toEqual({ x: 25, y: 25, width: 20, height: 10 })
  })

  it("returns null when the element is gone", () => {
    const pos = { x: 0, y: 0 }
    expect(locate(layout, { kind: "element", pos, id: "pump" })).toBeNull()
    expect(locate(layout, { kind: "position", pos })).toBeNull()
  })
})
