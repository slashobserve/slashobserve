import { describe, expect, it } from "vitest"

import {
  gridLayout,
  layeredLayout,
  type LayoutEdge,
  type LayoutNode,
} from "./layout"

function nodes(...ids: string[]): LayoutNode[] {
  return ids.map((id) => ({ id, title: id }))
}

function edge(source: string, target: string): LayoutEdge {
  return { source, target }
}

function at(layout: ReturnType<typeof layeredLayout>, id: string) {
  const point = layout.get(id)
  if (!point) throw new Error(`no position for ${id}`)
  return point
}

describe("layeredLayout", () => {
  it("places a chain in consecutive columns on one row", () => {
    const layout = layeredLayout(nodes("a", "b", "c"), [
      edge("a", "b"),
      edge("b", "c"),
    ])
    expect([...layout.values()]).toEqual(
      expect.arrayContaining([
        { x: 0, y: 0 },
        { x: 1, y: 0 },
        { x: 2, y: 0 },
      ])
    )
    expect(at(layout, "a").x).toBe(0)
    expect(at(layout, "b").x).toBe(1)
    expect(at(layout, "c").x).toBe(2)
  })

  it("spreads a fan-out around the source row", () => {
    const layout = layeredLayout(nodes("root", "x", "y", "z"), [
      edge("root", "x"),
      edge("root", "y"),
      edge("root", "z"),
    ])
    expect(at(layout, "root")).toEqual({ x: 0, y: 0 })
    const ys = ["x", "y", "z"].map((id) => at(layout, id).y)
    expect(ys.toSorted((a, b) => a - b)).toEqual([-1, 0, 1])
    for (const id of ["x", "y", "z"]) expect(at(layout, id).x).toBe(1)
  })

  it("pushes a node past its deepest parent", () => {
    const layout = layeredLayout(nodes("a", "b", "c"), [
      edge("a", "b"),
      edge("b", "c"),
      edge("a", "c"),
    ])
    expect(at(layout, "c").x).toBe(2)
  })

  it("lays out a cycle by reversing the edge that closes it", () => {
    const layout = layeredLayout(nodes("a", "b", "c"), [
      edge("a", "b"),
      edge("b", "c"),
      edge("c", "a"),
    ])
    expect(layout.size).toBe(3)
    expect([at(layout, "a").x, at(layout, "b").x, at(layout, "c").x]).toEqual([
      0, 1, 2,
    ])
  })

  it("keeps disconnected parts apart on separate rows of the first column", () => {
    const layout = layeredLayout(nodes("a", "b", "c", "d"), [
      edge("a", "b"),
      edge("c", "d"),
    ])
    expect(at(layout, "a").x).toBe(0)
    expect(at(layout, "c").x).toBe(0)
    expect(at(layout, "b").x).toBe(1)
    expect(at(layout, "d").x).toBe(1)
    expect(at(layout, "a").y).not.toBe(at(layout, "c").y)
  })

  it("orders a layer by the rows of its parents to avoid crossings", () => {
    const layout = layeredLayout(nodes("a", "b", "y", "x"), [
      edge("a", "x"),
      edge("b", "y"),
    ])
    const aboveFirst = at(layout, "a").y < at(layout, "b").y
    expect(at(layout, "x").y < at(layout, "y").y).toBe(aboveFirst)
  })

  it("ignores edges to unknown nodes and self edges", () => {
    const layout = layeredLayout(nodes("a", "b"), [
      edge("a", "ghost"),
      edge("b", "b"),
    ])
    expect([...layout.keys()].toSorted()).toEqual(["a", "b"])
    expect(at(layout, "a").x).toBe(0)
    expect(at(layout, "b").x).toBe(0)
  })

  it("returns the same positions for the same input", () => {
    const ids = Array.from({ length: 40 }, (_, i) => `n${i}`)
    const links = ids.flatMap((id, i) => [
      edge(id, ids[(i * 7 + 3) % ids.length]),
      edge(id, ids[(i * 11 + 5) % ids.length]),
    ])
    const first = layeredLayout(nodes(...ids), links)
    const second = layeredLayout(nodes(...ids), links)
    expect([...second]).toEqual([...first])
  })

  it("lays out 300 nodes and 1000 edges within 200 ms", () => {
    const ids = Array.from({ length: 300 }, (_, i) => `n${i}`)
    const links = Array.from({ length: 1000 }, (_, i) =>
      edge(ids[(i * 13) % 300], ids[(i * 29 + 7) % 300])
    )
    const started = performance.now()
    const layout = layeredLayout(nodes(...ids), links)
    expect(performance.now() - started).toBeLessThan(200)
    expect(layout.size).toBe(300)
  })
})

describe("gridLayout", () => {
  it("fills rows of ceil(sqrt(n)) nodes in title order", () => {
    const layout = gridLayout([
      { id: "1", title: "delta" },
      { id: "2", title: "alpha" },
      { id: "3", title: "charlie" },
      { id: "4", title: "bravo" },
      { id: "5", title: "echo" },
    ])
    expect(at(layout, "2")).toEqual({ x: 0, y: 0 })
    expect(at(layout, "4")).toEqual({ x: 1, y: 0 })
    expect(at(layout, "3")).toEqual({ x: 2, y: 0 })
    expect(at(layout, "1")).toEqual({ x: 0, y: 1 })
    expect(at(layout, "5")).toEqual({ x: 1, y: 1 })
  })

  it("breaks title ties by id and is deterministic", () => {
    const input = [
      { id: "b", title: "same" },
      { id: "a", title: "same" },
    ]
    expect([...gridLayout(input)]).toEqual([...gridLayout(input)])
    expect(at(gridLayout(input), "a")).toEqual({ x: 0, y: 0 })
  })

  it("returns nothing for no nodes", () => {
    expect(gridLayout([]).size).toBe(0)
  })
})
