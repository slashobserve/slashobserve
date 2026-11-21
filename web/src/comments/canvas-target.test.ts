import { describe, expect, it } from "vitest"

import type { Anchor, AnchorsModule, Point, Rect } from "@/panels/anchors"

import {
  outlineOf,
  placePin,
  pointerAnchor,
  titleAnchor,
  withWindow,
  type CanvasSurface,
} from "./canvas-target"

const body = { width: 400, height: 200 }

const bar: Rect = { x: 100, y: 40, width: 40, height: 120 }

const module: AnchorsModule = {
  hitTest: ((layout: { hit: boolean }, point: Point) =>
    layout.hit
      ? {
          kind: "bar",
          pos: { x: point.x / 400, y: point.y / 200 },
          category: "checkout",
        }
      : null) as AnchorsModule["hitTest"],
  locate: ((layout: { bar: Rect | null }, anchor: Anchor) =>
    anchor.category === "checkout" ? layout.bar : null) as NonNullable<
    AnchorsModule["locate"]
  >,
}

function surface(layout: { hit?: boolean; bar?: Rect | null }): CanvasSurface {
  return {
    module,
    published: { type: "barchart", layout: { hit: true, bar, ...layout } },
  }
}

const blank: CanvasSurface = { module: null, published: null }

const onBar: Anchor = {
  kind: "bar",
  pos: { x: 0.3, y: 0.1 },
  category: "checkout",
}

describe("pointerAnchor", () => {
  it("uses the panel's hit test", () => {
    expect(pointerAnchor(surface({}), { x: 120, y: 100 }, body)).toMatchObject({
      kind: "bar",
      category: "checkout",
    })
  })

  it("falls back to the spot when nothing is under the pointer", () => {
    expect(
      pointerAnchor(surface({ hit: false }), { x: 100, y: 50 }, body)
    ).toEqual({ kind: "position", pos: { x: 0.25, y: 0.25 } })
    expect(pointerAnchor(blank, { x: 100, y: 50 }, body)).toEqual({
      kind: "position",
      pos: { x: 0.25, y: 0.25 },
    })
  })

  it("pins a title anchor to the top edge", () => {
    expect(titleAnchor({ x: 200, y: -20 }, body)).toEqual({
      kind: "title",
      pos: { x: 0.5, y: 0 },
    })
  })
})

describe("outlineOf", () => {
  it("outlines the target the panel can locate", () => {
    expect(outlineOf(surface({}), onBar)).toEqual(bar)
  })

  it("outlines the whole panel otherwise", () => {
    expect(outlineOf(surface({}), null)).toBeNull()
    expect(
      outlineOf(surface({}), { kind: "position", pos: { x: 0, y: 0 } })
    ).toBeNull()
    expect(outlineOf(blank, onBar)).toBeNull()
  })
})

describe("placePin", () => {
  it("keeps the stored spot when it is inside the located target", () => {
    expect(
      placePin({ ...onBar, pos: { x: 0.3, y: 0.5 } }, body, surface({}), true)
    ).toEqual({ at: { x: 120, y: 100 }, gone: false })
  })

  it("moves the pin onto the target when the data moved", () => {
    expect(placePin(onBar, body, surface({}), true)).toEqual({
      at: { x: 120, y: 40 },
      gone: false,
    })
  })

  it("falls back to the stored spot when the data is gone", () => {
    expect(
      placePin({ ...onBar, category: "search" }, body, surface({}), true)
    ).toEqual({ at: { x: 120, y: 20 }, gone: true })
    expect(placePin(onBar, body, surface({}), false)).toEqual({
      at: { x: 120, y: 20 },
      gone: true,
    })
  })

  it("trusts the stored spot when the panel cannot locate anchors", () => {
    expect(placePin(onBar, body, blank, true)).toEqual({
      at: { x: 120, y: 20 },
      gone: false,
    })
  })

  it("never calls a spot or a title gone", () => {
    for (const kind of ["position", "title"] as const) {
      expect(
        placePin({ kind, pos: { x: 0.5, y: 0.5 } }, body, surface({}), false)
      ).toEqual({ at: { x: 200, y: 100 }, gone: false })
    }
  })
})

describe("withWindow", () => {
  it("records the time range the panel showed", () => {
    expect(withWindow(onBar, { from: "now-6h", to: "now" })).toEqual({
      ...onBar,
      window: { from: "now-6h", to: "now" },
    })
  })

  it("leaves the anchor alone without a full range", () => {
    expect(withWindow(onBar, { from: "", to: "now" })).toBe(onBar)
  })
})
