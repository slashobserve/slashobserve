import { describe, expect, it } from "vitest"
import type { Layout } from "react-grid-layout"
import { collides } from "react-grid-layout/core"

import type { Dashboard } from "@/spec"

import {
  applyLayout,
  layoutFromSpec,
  layoutPanels,
  moveItem,
  moveItemHorizontally,
  resizeItem,
  resizeItemPushing,
  settle,
} from "./layout"

function item(i: string, x: number, y: number, w: number, h: number) {
  return { i, x, y, w, h }
}

function find(layout: Layout, id: string) {
  const found = layout.find((entry) => entry.i === id)
  if (!found) throw new Error(`no item ${id}`)
  return found
}

function expectNoOverlap(layout: Layout) {
  for (const [index, a] of layout.entries()) {
    for (const b of layout.slice(index + 1)) {
      expect(collides(a, b), `${a.i} overlaps ${b.i}`).toBe(false)
    }
  }
}

describe("moveItem", () => {
  it("clamps to the grid", () => {
    const layout = [item("a", 20, 0, 4, 4)]
    expect(moveItem(layout, "a", 1, -1)).toEqual(layout)
  })
})

describe("resizeItem", () => {
  it("changes only the dimension being resized", () => {
    const tall = [item("a", 0, 0, 1, 60)]
    expect(resizeItem(tall, "a", 0, 1)).toEqual([item("a", 0, 0, 1, 61)])
    const wide = [item("a", 0, 0, 30, 4)]
    expect(resizeItem(wide, "a", 1, 0)).toEqual([item("a", 0, 0, 30, 4)])
    expect(resizeItem(wide, "a", -1, 0)).toEqual([item("a", 0, 0, 29, 4)])
  })

  it("stops at the minimum and at the right edge", () => {
    const layout = [item("a", 20, 0, 4, 4)]
    expect(resizeItem(layout, "a", 2, -5)).toEqual([item("a", 20, 0, 4, 3)])
    expect(resizeItem([item("a", 0, 0, 2, 3)], "a", -1, -1)).toEqual([
      item("a", 0, 0, 2, 3),
    ])
  })

  it("stops at the Spec maximum height of 100", () => {
    const layout = [item("a", 0, 0, 6, 99)]
    const once = resizeItem(layout, "a", 0, 1)
    expect(find(once, "a").h).toBe(100)
    expect(find(resizeItem(once, "a", 0, 1), "a").h).toBe(100)
  })

  it("lets the grid enforce the same maximum height for the mouse", () => {
    const [first] = layoutFromSpec({
      panels: [{ id: "a", gridPos: { x: 0, y: 0, w: 6, h: 4 } }],
    } as never)
    expect(first.maxH).toBe(100)
  })
})

describe("moveItemHorizontally", () => {
  it("pushes the neighbour in both directions", () => {
    const right = moveItemHorizontally(
      [item("a", 0, 0, 6, 4), item("b", 6, 0, 6, 4)],
      "a",
      1
    )
    expect(find(right, "a")).toMatchObject({ x: 1, y: 0 })
    expect(find(right, "b")).toMatchObject({ x: 6, y: 4 })

    const left = moveItemHorizontally(
      [item("a", 18, 0, 6, 4), item("b", 12, 0, 6, 4)],
      "a",
      -1
    )
    expect(find(left, "a")).toMatchObject({ x: 17, y: 0 })
    expect(find(left, "b")).toMatchObject({ x: 12, y: 4 })
    expectNoOverlap(right)
    expectNoOverlap(left)
  })

  it("returns the layout unchanged at the edge", () => {
    const layout = [item("a", 0, 0, 6, 4)]
    expect(moveItemHorizontally(layout, "a", -1)).toEqual(layout)
  })

  it("does not mutate its input", () => {
    const layout = [item("a", 0, 0, 6, 4), item("b", 6, 0, 6, 4)]
    const copy = structuredClone(layout)
    moveItemHorizontally(layout, "a", 1)
    expect(layout).toEqual(copy)
  })
})

describe("resizeItemPushing", () => {
  it("pushes a neighbour that starts on a higher row instead of dropping the resized panel", () => {
    const resized = resizeItemPushing(
      [item("c", 0, 0, 6, 4), item("a", 0, 4, 6, 4), item("b", 6, 0, 6, 8)],
      "a",
      1,
      0
    )
    expect(find(resized, "a")).toMatchObject({ x: 0, y: 4, w: 7, h: 4 })
    expect(find(resized, "b")).toMatchObject({ x: 6, y: 8 })
    expectNoOverlap(resized)
  })

  it("does not drop the resized panel below a taller neighbour", () => {
    const resized = resizeItemPushing(
      [item("a", 0, 4, 6, 4), item("b", 6, 0, 6, 8)],
      "a",
      1,
      0
    )
    expect(find(resized, "a").y).toBeLessThanOrEqual(4)
    expectNoOverlap(resized)
  })

  it("leaves other panels alone when nothing collides", () => {
    const layout = [item("a", 0, 0, 6, 4), item("b", 12, 0, 6, 4)]
    const resized = resizeItemPushing(layout, "a", 1, 1)
    expect(find(resized, "a")).toMatchObject({ w: 7, h: 5 })
    expect(find(resized, "b")).toMatchObject({ x: 12, y: 0 })
  })

  it("does not mutate its input", () => {
    const layout = [item("a", 0, 4, 6, 4), item("b", 6, 0, 6, 8)]
    const copy = structuredClone(layout)
    resizeItemPushing(layout, "a", 1, 0)
    expect(layout).toEqual(copy)
  })
})

describe("settle", () => {
  it("pulls panels up into gaps", () => {
    const settled = settle([item("a", 0, 0, 6, 4), item("b", 0, 10, 6, 4)])
    expect(find(settled, "b").y).toBe(4)
  })
})

function panel(id: string, x: number, y: number, w: number, h: number) {
  return {
    id,
    type: "stat",
    title: id,
    gridPos: { x, y, w, h },
    datasource: "prom",
    queries: [{ query: "up" }],
  }
}

function row(
  id: string,
  y: number,
  collapsed: boolean,
  children: ReturnType<typeof panel>[]
) {
  return {
    id,
    type: "row",
    title: id,
    collapsed,
    gridPos: { x: 0, y, w: 24, h: 1 },
    panels: children,
  }
}

function specOf(panels: unknown[]): Dashboard {
  return {
    specVersion: "0",
    title: "Rows",
    time: { from: "now-1h", to: "now" },
    panels,
  } as Dashboard
}

describe("rows in the layout", () => {
  const expanded = specOf([
    panel("top", 0, 0, 12, 4),
    row("r", 4, false, [panel("c1", 0, 5, 12, 8), panel("c2", 12, 5, 12, 8)]),
  ])
  const collapsed = specOf([
    panel("top", 0, 0, 12, 4),
    row("r", 4, true, [panel("c1", 0, 5, 12, 8), panel("c2", 12, 5, 12, 8)]),
    panel("below", 0, 5, 24, 4),
  ])

  it("lays out a row as a static full-width item", () => {
    expect(find(layoutFromSpec(expanded), "r")).toEqual({
      i: "r",
      x: 0,
      y: 4,
      w: 24,
      h: 1,
      static: true,
    })
  })

  it("lays out the children of an expanded row and not those of a collapsed row", () => {
    expect(layoutPanels(expanded).map((entry) => entry.id)).toEqual([
      "top",
      "r",
      "c1",
      "c2",
    ])
    expect(layoutPanels(collapsed).map((entry) => entry.id)).toEqual([
      "top",
      "r",
      "below",
    ])
  })

  it("keeps expanded children below their row after compaction", () => {
    const settled = settle(layoutFromSpec(expanded))
    expect(find(settled, "r")).toMatchObject({ y: 4, h: 1 })
    expect(find(settled, "c1").y).toBe(5)
    expect(find(settled, "c2").y).toBe(5)
    expectNoOverlap(settled)
  })

  it("pushes children clear of a taller panel above the row", () => {
    const grown = resizeItemPushing(
      settle(layoutFromSpec(expanded)),
      "top",
      0,
      3
    )
    expect(find(grown, "r").y).toBe(4)
    expect(find(grown, "c1").y).toBeGreaterThanOrEqual(5)
    expectNoOverlap(grown)
  })

  it("saves an expanded row layout without overlap and leaves the row alone", () => {
    const saved = applyLayout(expanded, settle(layoutFromSpec(expanded)))
    expect(saved).toEqual(expanded)
    const moved = applyLayout(
      expanded,
      moveItem(settle(layoutFromSpec(expanded)), "top", 6, 0)
    )
    const rowPanel = moved.panels[1]
    expect(rowPanel.gridPos).toEqual({ x: 0, y: 4, w: 24, h: 1 })
    expect(moved.panels[0].gridPos).toMatchObject({ x: 6, y: 0 })
  })

  it("saves a layout pushed around a row without overlap", () => {
    const layout = resizeItemPushing(
      settle(layoutFromSpec(expanded)),
      "top",
      0,
      3
    )
    const saved = applyLayout(expanded, layout)
    expect(saved.panels[1].gridPos).toEqual({ x: 0, y: 4, w: 24, h: 1 })
    expectNoOverlap(layoutFromSpec(saved))
  })

  it("leaves the children of a collapsed row at their stored positions", () => {
    const layout = moveItem(settle(layoutFromSpec(collapsed)), "below", 0, 3)
    const saved = applyLayout(collapsed, layout)
    const savedRow = saved.panels[1] as unknown as {
      panels: { gridPos: unknown }[]
    }
    expect(savedRow.panels.map((child) => child.gridPos)).toEqual([
      { x: 0, y: 5, w: 12, h: 8 },
      { x: 12, y: 5, w: 12, h: 8 },
    ])
    expect(saved.panels[1].gridPos).toEqual({ x: 0, y: 4, w: 24, h: 1 })
    expectNoOverlap(layoutFromSpec(saved))
  })
})
