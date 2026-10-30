import { describe, expect, it } from "vitest"

import { forceLayout } from "./force-layout"
import type { Layout, LayoutEdge, LayoutNode } from "./layout"

const area = { width: 800, height: 480, seed: 7 }

function nodes(...ids: string[]): LayoutNode[] {
  return ids.map((id) => ({ id, title: id }))
}

function edge(source: string, target: string): LayoutEdge {
  return { source, target }
}

function distance(layout: Layout, a: string, b: string): number {
  const from = layout.get(a)
  const to = layout.get(b)
  if (!from || !to) throw new Error(`no position for ${a} or ${b}`)
  return Math.hypot(from.x - to.x, from.y - to.y)
}

function everyPointIsFinite(layout: Layout): boolean {
  return [...layout.values()].every(
    ({ x, y }) => Number.isFinite(x) && Number.isFinite(y)
  )
}

function ring(count: number): [LayoutNode[], LayoutEdge[]] {
  const ids = Array.from({ length: count }, (_, at) => `n${at}`)
  return [nodes(...ids), ids.map((id, at) => edge(id, ids[(at + 1) % count]))]
}

describe("forceLayout", () => {
  it("gives the same positions for the same input", () => {
    const graph = nodes("a", "b", "c", "d", "e")
    const links = [edge("a", "b"), edge("b", "c"), edge("c", "d")]
    expect(forceLayout(graph, links, area)).toEqual(
      forceLayout(graph, links, area)
    )
  })

  it("changes the picture with the seed", () => {
    const graph = nodes("a", "b", "c")
    const links = [edge("a", "b")]
    expect(forceLayout(graph, links, area)).not.toEqual(
      forceLayout(graph, links, { ...area, seed: 8 })
    )
  })

  it("draws connected nodes closer than unconnected ones", () => {
    const ids = ["a", "b", "c", "d", "e", "f", "g", "h"]
    const links = [
      edge("a", "b"),
      edge("b", "c"),
      edge("c", "d"),
      edge("e", "f"),
      edge("f", "g"),
      edge("g", "h"),
    ]
    const layout = forceLayout(nodes(...ids), links, area)
    const joined = new Set(
      links.map(({ source, target }) => `${source}${target}`)
    )
    const near: number[] = []
    const far: number[] = []
    for (let a = 0; a < ids.length; a++) {
      for (let b = a + 1; b < ids.length; b++) {
        const gap = distance(layout, ids[a], ids[b])
        ;(joined.has(`${ids[a]}${ids[b]}`) ? near : far).push(gap)
      }
    }
    const mean = (values: number[]) =>
      values.reduce((sum, value) => sum + value, 0) / values.length
    expect(mean(near)).toBeLessThan(mean(far) / 2)
  })

  it("places no node on top of another", () => {
    const [graph, links] = ring(30)
    const layout = forceLayout(graph, links, area)
    for (let a = 0; a < graph.length; a++) {
      for (let b = a + 1; b < graph.length; b++) {
        expect(distance(layout, graph[a].id, graph[b].id)).toBeGreaterThan(1)
      }
    }
  })

  it.each([
    ["no nodes", nodes(), 0],
    ["one node", nodes("a"), 1],
    ["two nodes", nodes("a", "b"), 2],
  ])("returns finite positions for %s", (_, graph, size) => {
    const layout = forceLayout(graph, [edge("a", "b")], area)
    expect(layout.size).toBe(size)
    expect(everyPointIsFinite(layout)).toBe(true)
  })

  it("centers a lone node", () => {
    expect(forceLayout(nodes("a"), [], area).get("a")).toEqual({
      x: area.width / 2,
      y: area.height / 2,
    })
  })

  it("ignores self loops, repeated edges and edges to unknown nodes", () => {
    const layout = forceLayout(
      nodes("a", "b"),
      [edge("a", "a"), edge("a", "b"), edge("a", "b"), edge("a", "zzz")],
      area
    )
    expect(layout.size).toBe(2)
    expect(everyPointIsFinite(layout)).toBe(true)
  })

  it("survives nodes that start on the same spot", () => {
    const layout = forceLayout(nodes("a", "b", "c"), [], {
      width: 0,
      height: 0,
      seed: 1,
    })
    expect(everyPointIsFinite(layout)).toBe(true)
  })

  it("is deterministic and finite above the tree threshold", () => {
    const [graph, links] = ring(400)
    const first = forceLayout(graph, links, area)
    expect(first).toEqual(forceLayout(graph, links, area))
    expect(everyPointIsFinite(first)).toBe(true)
  })

  it("lays out 1,000 nodes in under 2 s", () => {
    const [graph, links] = ring(1000)
    const started = performance.now()
    const layout = forceLayout(graph, links, {
      width: 4000,
      height: 2400,
      seed: 3,
    })
    const elapsed = performance.now() - started
    expect(everyPointIsFinite(layout)).toBe(true)
    expect(elapsed).toBeLessThan(2000)
  })
})
