import { describe, expect, it } from "vitest"
import type { Layout } from "react-grid-layout"
import { collides } from "react-grid-layout/core"

import type { Dashboard } from "@/spec"

import {
  applyLayout,
  layoutFromSpec,
  layoutPanels,
  moveItem,
  membershipOf,
  moveItemHorizontally,
  resizeItem,
  resizeItemPushing,
  sameLayout,
  settle,
} from "./layout"

const flat = new Map<string, string | null>()

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
      flat,
      "a",
      1
    )
    expect(find(right, "a")).toMatchObject({ x: 1, y: 0 })
    expect(find(right, "b")).toMatchObject({ x: 6, y: 4 })

    const left = moveItemHorizontally(
      [item("a", 18, 0, 6, 4), item("b", 12, 0, 6, 4)],
      flat,
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
    expect(moveItemHorizontally(layout, flat, "a", -1)).toEqual(layout)
  })

  it("does not mutate its input", () => {
    const layout = [item("a", 0, 0, 6, 4), item("b", 6, 0, 6, 4)]
    const copy = structuredClone(layout)
    moveItemHorizontally(layout, flat, "a", 1)
    expect(layout).toEqual(copy)
  })
})

describe("resizeItemPushing", () => {
  it("pushes a neighbour that starts on a higher row instead of dropping the resized panel", () => {
    const resized = resizeItemPushing(
      [item("c", 0, 0, 6, 4), item("a", 0, 4, 6, 4), item("b", 6, 0, 6, 8)],
      flat,
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
      flat,
      "a",
      1,
      0
    )
    expect(find(resized, "a").y).toBeLessThanOrEqual(4)
    expectNoOverlap(resized)
  })

  it("leaves other panels alone when nothing collides", () => {
    const layout = [item("a", 0, 0, 6, 4), item("b", 12, 0, 6, 4)]
    const resized = resizeItemPushing(layout, flat, "a", 1, 1)
    expect(find(resized, "a")).toMatchObject({ w: 7, h: 5 })
    expect(find(resized, "b")).toMatchObject({ x: 12, y: 0 })
  })

  it("does not mutate its input", () => {
    const layout = [item("a", 0, 4, 6, 4), item("b", 6, 0, 6, 8)]
    const copy = structuredClone(layout)
    resizeItemPushing(layout, flat, "a", 1, 0)
    expect(layout).toEqual(copy)
  })
})

describe("settle", () => {
  it("pulls panels up into gaps", () => {
    const settled = settle(
      [item("a", 0, 0, 6, 4), item("b", 0, 10, 6, 4)],
      flat
    )
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
    const settled = settle(layoutFromSpec(expanded), membershipOf(expanded))
    expect(find(settled, "r")).toMatchObject({ y: 4, h: 1 })
    expect(find(settled, "c1").y).toBe(5)
    expect(find(settled, "c2").y).toBe(5)
    expectNoOverlap(settled)
  })

  it("shifts the row and its children below a taller panel above the row", () => {
    const grown = resizeItemPushing(
      settle(layoutFromSpec(expanded), membershipOf(expanded)),
      membershipOf(expanded),
      "top",
      0,
      3
    )
    expect(find(grown, "r").y).toBe(7)
    expect(find(grown, "c1").y).toBe(8)
    expectNoOverlap(grown)
  })

  it("saves an expanded row layout without overlap and leaves the row alone", () => {
    const membership = membershipOf(expanded)
    const saved = applyLayout(
      expanded,
      settle(layoutFromSpec(expanded), membership)
    )
    expect(saved).toEqual(expanded)
    const moved = applyLayout(
      expanded,
      moveItem(settle(layoutFromSpec(expanded), membership), "top", 6, 0)
    )
    const rowPanel = moved.panels[1]
    expect(rowPanel.gridPos).toEqual({ x: 0, y: 4, w: 24, h: 1 })
    expect(moved.panels[0].gridPos).toMatchObject({ x: 6, y: 0 })
  })

  it("saves a layout pushed around a row without overlap", () => {
    const membership = membershipOf(expanded)
    const layout = resizeItemPushing(
      settle(layoutFromSpec(expanded), membership),
      membership,
      "top",
      0,
      3
    )
    const saved = applyLayout(expanded, layout)
    expect(saved.panels[1].gridPos).toEqual({ x: 0, y: 7, w: 24, h: 1 })
    expectNoOverlap(layoutFromSpec(saved))
  })

  it("leaves the children of a collapsed row at their stored positions", () => {
    const layout = moveItem(
      settle(layoutFromSpec(collapsed), membershipOf(collapsed)),
      "below",
      0,
      3
    )
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

describe("resizing inside a row", () => {
  const spec = specOf([
    row("r1", 0, false, [
      panel("pressure", 0, 1, 3, 4),
      panel("busy", 3, 1, 3, 4),
    ]),
    row("r2", 5, false, [
      panel("basic", 0, 6, 12, 7),
      panel("memory", 12, 6, 12, 7),
    ]),
    row("r3", 13, true, [panel("hidden", 0, 14, 12, 12)]),
    row("r4", 14, true, [panel("hiddenToo", 0, 15, 12, 12)]),
  ])
  const membership = membershipOf(spec)
  const baseline = settle(layoutFromSpec(spec), membership)

  it("keeps a panel in its row when it grows taller and shifts the rows below", () => {
    const grown = resizeItemPushing(baseline, membership, "pressure", 0, 3)
    expect(find(grown, "pressure")).toMatchObject({ y: 1, h: 7 })
    expect(find(grown, "r2").y).toBe(8)
    expect(find(grown, "basic").y).toBe(9)
    expect(find(grown, "memory").y).toBe(9)
    expect(find(grown, "r3").y).toBe(16)
    expect(find(grown, "r4").y).toBe(17)
    expectNoOverlap(grown)
  })

  it("keeps a panel in its row when the grid grows it past the next row header", () => {
    const grown = settle(resizeItem(baseline, "pressure", 0, 3), membership)
    expect(find(grown, "pressure")).toMatchObject({ y: 1, h: 7 })
    expect(find(grown, "r2").y).toBe(8)
    expect(find(grown, "basic").y).toBe(9)
    expectNoOverlap(grown)
  })

  it("pushes the neighbour down inside the row when it grows wider", () => {
    const widened = resizeItemPushing(baseline, membership, "pressure", 3, 0)
    expect(find(widened, "pressure")).toMatchObject({ y: 1, w: 6 })
    expect(find(widened, "busy").y).toBe(5)
    expect(find(widened, "r2").y).toBe(9)
    expect(find(widened, "basic").y).toBe(10)
    expectNoOverlap(widened)
  })

  it("keeps the last panel of a row in its row", () => {
    const grown = resizeItemPushing(baseline, membership, "memory", 0, 2)
    expect(find(grown, "memory")).toMatchObject({ y: 6, h: 9 })
    expect(find(grown, "r3").y).toBe(15)
    expect(find(grown, "r4").y).toBe(16)
    expectNoOverlap(grown)
  })

  it("pulls the rows below up when a panel shrinks", () => {
    const small = settle(
      resizeItem(resizeItem(baseline, "pressure", 0, -1), "busy", 0, -1),
      membership
    )
    expect(find(small, "r2").y).toBe(4)
    expect(find(small, "basic").y).toBe(5)
    expect(find(small, "r4").y).toBe(13)
  })

  it("keeps a panel dragged below its row at the bottom of its own row", () => {
    const dragged = settle(moveItem(baseline, "pressure", 0, 9), membership)
    expect(find(dragged, "pressure").y).toBe(1)
    expect(find(dragged, "r2").y).toBe(5)
    expect(find(dragged, "basic").y).toBe(6)
    expectNoOverlap(dragged)
  })

  it("keeps a panel dragged above its row header just below the header", () => {
    const dragged = settle(moveItem(baseline, "basic", 0, -4), membership)
    expect(find(dragged, "r2").y).toBe(5)
    expect(find(dragged, "basic").y).toBe(6)
  })

  it("moves a panel sideways inside its row without leaving it", () => {
    const moved = moveItemHorizontally(baseline, membership, "pressure", 1)
    expect(find(moved, "pressure")).toMatchObject({ x: 1, y: 1 })
    expect(find(moved, "r2").y).toBeGreaterThan(find(moved, "pressure").y)
    expectNoOverlap(moved)
  })

  it("writes the shifted rows and their collapsed children into the saved Spec", () => {
    const saved = applyLayout(
      spec,
      resizeItemPushing(baseline, membership, "pressure", 0, 3)
    )
    const savedMembership = membershipOf(saved)
    const rows = saved.panels as unknown as {
      id: string
      gridPos: { y: number }
      panels: { id: string; gridPos: { y: number; h: number } }[]
    }[]
    expect(rows.map((entry) => entry.gridPos.y)).toEqual([0, 8, 16, 17])
    expect(rows[0].panels[0].gridPos).toMatchObject({ y: 1, h: 7 })
    expect(rows[1].panels.map((child) => child.gridPos.y)).toEqual([9, 9])
    expect(rows[2].panels[0].gridPos.y).toBe(17)
    expect(rows[3].panels[0].gridPos.y).toBe(18)
    expect(
      sameLayout(
        settle(layoutFromSpec(saved), savedMembership),
        layoutFromSpec(saved)
      )
    ).toBe(true)
  })

  it("writes nothing for a layout that did not change", () => {
    expect(applyLayout(spec, baseline)).toEqual(spec)
  })
})

describe("resizing a panel above the first row", () => {
  const spec = specOf([
    panel("top", 0, 0, 12, 4),
    row("r", 4, false, [panel("c", 0, 5, 12, 8)]),
  ])

  it("shifts the row and its children down", () => {
    const membership = membershipOf(spec)
    const grown = resizeItemPushing(
      settle(layoutFromSpec(spec), membership),
      membership,
      "top",
      0,
      3
    )
    expect(find(grown, "top")).toMatchObject({ y: 0, h: 7 })
    expect(find(grown, "r").y).toBe(7)
    expect(find(grown, "c").y).toBe(8)
  })
})

describe("moving a panel sideways above the first row", () => {
  const spec = specOf([
    panel("logs", 0, 0, 12, 4),
    panel("note", 12, 0, 12, 4),
    row("r", 4, false, [
      panel("up", 0, 5, 12, 8),
      panel("world", 12, 5, 12, 8),
    ]),
  ])
  const membership = membershipOf(spec)

  it("pushes the neighbour it overlaps below it and shifts the row by that height", () => {
    const moved = moveItemHorizontally(
      settle(layoutFromSpec(spec), membership),
      membership,
      "logs",
      1
    )
    expect(find(moved, "logs")).toMatchObject({ x: 1, y: 0 })
    expect(find(moved, "note")).toMatchObject({ x: 12, y: 4 })
    expect(find(moved, "r").y).toBe(8)
    expect(find(moved, "up").y).toBe(9)
    expectNoOverlap(moved)
  })

  it("leaves the row in place when the move overlaps nothing", () => {
    const apart = specOf([
      panel("logs", 0, 0, 10, 4),
      panel("note", 12, 0, 12, 4),
      row("r", 4, false, [panel("up", 0, 5, 12, 8)]),
    ])
    const apartMembership = membershipOf(apart)
    const moved = moveItemHorizontally(
      settle(layoutFromSpec(apart), apartMembership),
      apartMembership,
      "logs",
      1
    )
    expect(find(moved, "logs")).toMatchObject({ x: 1, y: 0 })
    expect(find(moved, "r").y).toBe(4)
    expect(find(moved, "up").y).toBe(5)
  })
})
