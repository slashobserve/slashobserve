import { afterEach, describe, expect, it, vi } from "vitest"

import { panelOf, renderPublishing, stubRects } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
import type { Rect } from "../anchors"
import type { CanvasAnchorLayout } from "./anchors"
import CanvasPanel from "./canvas-panel"

const frames: PreparedFrame[] = [
  {
    ref: "A",
    name: "orders",
    labels: {},
    fields: [
      { name: "Time", type: "time", values: [1, 2] },
      { name: "Value", type: "number", values: [3, 4] },
    ],
  } as PreparedFrame,
]

const elements = [
  {
    id: "frame",
    kind: "rectangle",
    left: 5,
    top: 5,
    width: 60,
    height: 40,
  },
  {
    id: "orders",
    kind: "metric",
    left: 10,
    top: 10,
    width: 20,
    height: 20,
    text: "Orders",
  },
]

function panel(display: Record<string, unknown> = {}) {
  return panelOf("canvas", { elements, ...display })
}

const rects: Record<string, Rect> = {
  frame: { x: 5, y: 5, width: 60, height: 40 },
  orders: { x: 10, y: 10, width: 20, height: 20 },
  touching: { x: 400, y: 50, width: 30, height: 20 },
  straddling: { x: 380, y: 100, width: 60, height: 20 },
  away: { x: 500, y: 50, width: 30, height: 20 },
}

function stubCanvas() {
  stubRects((element) =>
    element.getAttribute("role") === "group"
      ? { x: 0, y: 0, width: 400, height: 240 }
      : rects[(element as HTMLElement).dataset.elementId ?? ""]
  )
}

afterEach(() => vi.restoreAllMocks())

describe("canvas anchors", () => {
  it("keeps only the part of an element inside the canvas and skips one that only touches its edge", () => {
    stubCanvas()
    const extra = ["touching", "straddling", "away"].map((id) => ({
      id,
      kind: "metric",
      left: 0,
      top: 0,
      width: 10,
      height: 10,
      text: id,
    }))
    const { store } = renderPublishing(
      CanvasPanel,
      panel({ elements: [...elements, ...extra] }),
      frames
    )
    const layout = store.current()?.layout as CanvasAnchorLayout
    expect(
      Object.fromEntries(layout.elements.map(({ id, rect }) => [id, rect]))
    ).toEqual({
      frame: rects.frame,
      orders: rects.orders,
      straddling: { x: 380, y: 100, width: 20, height: 20 },
    })
  })

  it("publishes each element it drew", () => {
    stubCanvas()
    const { store } = renderPublishing(CanvasPanel, panel(), frames)
    const published = store.current()
    expect(published?.type).toBe("canvas")
    const layout = published?.layout as CanvasAnchorLayout
    expect(layout.body).toEqual({ width: 400, height: 240 })
    expect(new Set(layout.elements.map(({ id }) => id))).toEqual(
      new Set(["frame", "orders"])
    )
  })

  it("publishes nothing for a canvas with no elements", () => {
    const { store } = renderPublishing(CanvasPanel, panel({ elements: [] }), [])
    expect(store.current()).toBeNull()
  })

  it("withdraws its layout when it unmounts", () => {
    const { store, view } = renderPublishing(CanvasPanel, panel(), frames)
    view.unmount()
    expect(store.current()).toBeNull()
  })
})
