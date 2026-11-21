import { describe, expect, it } from "vitest"

import { edgeHeight } from "../list-anchors"

import { hitTest, locate, type SpanAnchorLayout } from "./anchors"

const layout: SpanAnchorLayout = {
  body: { width: 400, height: 100 },
  spans: [
    {
      rect: { x: 0, y: 0, width: 400, height: 20 },
      span: { id: "a1", service: "checkout", operation: "process_order" },
    },
    {
      rect: { x: 0, y: 20, width: 400, height: 20 },
      span: { id: "b2", service: "payments", operation: "" },
    },
  ],
}

describe("traces hitTest", () => {
  it("names the span under the pointer", () => {
    expect(hitTest(layout, { x: 40, y: 10 })).toEqual({
      kind: "span",
      pos: { x: 0.1, y: 0.1 },
      id: "a1",
      quote: "checkout process_order",
    })
  })

  it("quotes only the parts a span has", () => {
    expect(hitTest(layout, { x: 40, y: 30 })).toMatchObject({
      id: "b2",
      quote: "payments",
    })
  })

  it("returns null where no span is drawn", () => {
    expect(hitTest(layout, { x: 40, y: 80 })).toBeNull()
  })
})

describe("traces locate", () => {
  it("finds the span a hit test named", () => {
    const anchor = hitTest(layout, { x: 40, y: 30 })
    expect(anchor).not.toBeNull()
    expect(locate(layout, anchor!)).toEqual(layout.spans[1].rect)
  })

  it("points at the bottom edge for a span scrolled out of view", () => {
    const scrolled = { ...layout, elsewhere: ["c3"] }
    expect(
      locate(scrolled, { kind: "span", pos: { x: 0, y: 0 }, id: "c3" })
    ).toEqual({ x: 0, y: 100 - edgeHeight, width: 400, height: edgeHeight })
  })

  it("returns null when the span is gone or the anchor is not a span", () => {
    expect(
      locate(layout, { kind: "span", pos: { x: 0, y: 0 }, id: "c3" })
    ).toBeNull()
    expect(locate(layout, { kind: "position", pos: { x: 0, y: 0 } })).toBeNull()
  })
})
