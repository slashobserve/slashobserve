import { describe, expect, it } from "vitest"

import { panelOf, renderPublishing } from "../anchor-harness"
import type { PreparedFrame } from "../transforms"
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

describe("canvas anchors", () => {
  it("publishes each element it drew", () => {
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
