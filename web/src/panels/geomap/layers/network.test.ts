import { describe, expect, it } from "vitest"

import { project } from "../projection"
import {
  idText,
  maxEdges,
  networkModel,
  truncatedEdgesText,
  type EdgeRow,
  type NetworkNode,
} from "./network"

const nodes: NetworkNode[] = [
  { id: "a", lat: 10, lon: 20 },
  { id: "b", lat: 30, lon: 40 },
  { id: "7", lat: -10, lon: -20 },
]

function edge(row: number, source: unknown, target: unknown): EdgeRow {
  return { frameIndex: 0, row, source, target }
}

describe("networkModel", () => {
  it("joins located nodes with a line between their places", () => {
    const model = networkModel(nodes, [edge(0, "a", "b")], {}, undefined)
    expect(model.edges).toHaveLength(1)
    expect(model.edges[0].from).toEqual(project(20, 10))
    expect(model.edges[0].to).toEqual(project(40, 30))
  })

  it("builds edges only between located nodes and counts the rest", () => {
    const model = networkModel(
      nodes,
      [edge(0, "a", "b"), edge(1, "a", "nowhere"), edge(2, "ghost", "b")],
      {},
      undefined
    )
    expect(model.edges.map((e) => e.key)).toEqual(["0:0"])
    expect(model.unlocated).toBe(2)
  })

  it("matches numeric ids against text ids", () => {
    const model = networkModel(nodes, [edge(0, 7, "a")], {}, undefined)
    expect(model.edges).toHaveLength(1)
  })

  it("ignores edges with an empty or missing end", () => {
    const model = networkModel(
      nodes,
      [edge(0, "", "a"), edge(1, null, "a"), edge(2, "a", undefined)],
      {},
      undefined
    )
    expect(model.edges).toHaveLength(0)
    expect(model.unlocated).toBe(3)
  })

  it("keeps at most 1,000 edges and counts the rest", () => {
    const rows = Array.from({ length: maxEdges + 40 }, (_, row) =>
      edge(row, "a", "b")
    )
    const model = networkModel(nodes, rows, {}, undefined)
    expect(model.edges).toHaveLength(1000)
    expect(model.truncated).toBe(40)
    expect(truncatedEdgesText(40)).toBe(
      "40 more connections were left off the map."
    )
  })

  it("reads the line width, arrow and chosen color", () => {
    expect(networkModel([], [], {}, undefined)).toMatchObject({
      lineWidth: 1,
      arrow: "none",
      color: null,
    })
    expect(
      networkModel([], [], { lineWidth: 3, arrow: "forward" }, "var(--x)")
    ).toMatchObject({ lineWidth: 3, arrow: "forward", color: "var(--x)" })
  })
})

describe("idText", () => {
  it("reads text and numbers and nothing else", () => {
    expect(idText("a")).toBe("a")
    expect(idText(12)).toBe("12")
    expect(idText("")).toBeNull()
    expect(idText(null)).toBeNull()
    expect(idText(NaN)).toBeNull()
    expect(idText({})).toBeNull()
  })
})
