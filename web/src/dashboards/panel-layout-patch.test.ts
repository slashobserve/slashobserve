import { describe, expect, it } from "vitest"

import type { Dashboard, LayoutPanel } from "@/spec"

import {
  copyIdFor,
  duplicatePanelPatch,
  removePanelPatch,
} from "./panel-layout-patch"

function text(id: string, x: number, y: number, w = 12, h = 4): LayoutPanel {
  return {
    id,
    type: "text",
    title: id.toUpperCase(),
    gridPos: { x, y, w, h },
    content: id,
  } as LayoutPanel
}

function dashboard(panels: Dashboard["panels"]): Dashboard {
  return {
    specVersion: "0",
    title: "Shop",
    time: { from: "now-1h", to: "now" },
    panels,
  } as Dashboard
}

describe("duplicatePanelPatch", () => {
  it("places a titled copy under the panel and moves what starts below it down", () => {
    const a = text("a", 0, 0)
    const beside = text("beside", 12, 0, 12, 8)
    const below = text("below", 0, 4)
    const patch = duplicatePanelPatch(dashboard([a, beside, below]), a)
    expect(patch.upsert).toEqual([
      {
        ...a,
        id: "a-copy",
        title: "A copy",
        gridPos: { x: 0, y: 4, w: 12, h: 4 },
      },
    ])
    expect(patch.layout).toEqual([
      { id: "below", gridPos: { x: 0, y: 8, w: 12, h: 4 } },
    ])
  })

  it("keeps a copy of a row child in that row", () => {
    const child = text("child", 0, 1)
    const row = {
      id: "r1",
      type: "row",
      title: "Row",
      gridPos: { x: 0, y: 0, w: 24, h: 1 },
      panels: [child],
    } as Dashboard["panels"][number]
    const patch = duplicatePanelPatch(dashboard([row]), child)
    expect(patch.layout).toEqual([{ id: "child-copy", row: "r1" }])
  })

  it("moves a collapsed row's hidden panels with the row", () => {
    const a = text("a", 0, 0)
    const hidden = text("hidden", 0, 6)
    const row = {
      id: "r1",
      type: "row",
      title: "Row",
      collapsed: true,
      gridPos: { x: 0, y: 5, w: 24, h: 1 },
      panels: [hidden],
    } as Dashboard["panels"][number]
    const patch = duplicatePanelPatch(dashboard([a, row]), a)
    expect(patch.layout).toEqual([
      { id: "r1", gridPos: { x: 0, y: 9, w: 24, h: 1 } },
      { id: "hidden", gridPos: { x: 0, y: 10, w: 12, h: 4 } },
    ])
  })
})

describe("copyIdFor", () => {
  it("picks the next free copy id within the 64 character limit", () => {
    const spec = dashboard([text("a", 0, 0), text("a-copy", 0, 4)])
    expect(copyIdFor(spec, "a")).toBe("a-copy-2")
    const long = "p".repeat(64)
    expect(copyIdFor(dashboard([]), long).length).toBeLessThanOrEqual(64)
  })
})

describe("removePanelPatch", () => {
  it("deletes the panel by id", () => {
    expect(removePanelPatch(text("a", 0, 0))).toEqual({ delete: ["a"] })
  })
})
