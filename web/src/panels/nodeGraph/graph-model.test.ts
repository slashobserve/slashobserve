import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedField, PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  edgeRows,
  graphModel,
  maxEdges,
  maxNodes,
  maxThickness,
  missingFramesReason,
  nodeRows,
} from "./graph-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "nodeGraph",
    datasource: "ch",
    queries: [{ query: "SELECT 1" }],
    ...extra,
  } as DataPanel
}

function field(
  name: string,
  type: PreparedField["type"],
  values: unknown[]
): PreparedField {
  return { name, type, values }
}

function frame(
  fields: PreparedField[],
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return { ref: "A", fields, ...extra }
}

function build(frames: PreparedFrame[], extra: Partial<DataPanel> = {}) {
  const model = graphModel(frames, createFieldResolver(panel(extra)))
  if (model.reason !== null) throw new Error(model.reason)
  return model
}

describe("graphModel from edges only", () => {
  const edges = frame([
    field("id", "string", ["e1", "e2"]),
    field("source", "string", ["web", "api"]),
    field("target", "string", ["api", "db"]),
    field("mainstat", "number", [12, 3]),
  ])

  it("creates a node for every endpoint, titled by its id", () => {
    const { nodes } = build([edges])
    expect(nodes.map((node) => [node.id, node.title])).toEqual([
      ["web", "web"],
      ["api", "api"],
      ["db", "db"],
    ])
  })

  it("reads edge ids, ends and the main stat through the field config", () => {
    const { edges: list } = build([edges], {
      overrides: [{ match: { field: "mainstat" }, unit: "ms" }],
    })
    expect(list.map((edge) => [edge.id, edge.source, edge.target])).toEqual([
      ["e1", "web", "api"],
      ["e2", "api", "db"],
    ])
    expect(list[0].mainStat?.display.text).toBe("12 ms")
    expect(list[0].thickness).toBe(1)
  })

  it("matches field names without regard to case", () => {
    const { edges: list } = build([
      frame([
        field("ID", "string", ["e"]),
        field("Source", "string", ["a"]),
        field("TARGET", "string", ["b"]),
      ]),
    ])
    expect(list).toHaveLength(1)
  })

  it("names an edge without an id after its ends and keeps duplicates apart", () => {
    const { edges: list } = build([
      frame([
        field("source", "string", ["a", "a"]),
        field("target", "string", ["b", "b"]),
      ]),
    ])
    expect(list.map((edge) => edge.id)).toEqual(["a->b", "a->b#1"])
  })

  it("skips rows missing an end", () => {
    const { edges: list } = build([
      frame([
        field("source", "string", ["a", null]),
        field("target", "string", ["b", "c"]),
      ]),
    ])
    expect(list).toHaveLength(1)
  })

  it("clamps thickness between 1 and 8", () => {
    const { edges: list } = build([
      frame([
        field("source", "string", ["a", "a", "a", "a"]),
        field("target", "string", ["b", "c", "d", "e"]),
        field("thickness", "number", [0, 3, 50, null]),
      ]),
    ])
    expect(list.map((edge) => edge.thickness)).toEqual([1, 3, maxThickness, 1])
  })
})

describe("graphModel from nodes and edges", () => {
  const nodeFrame = frame(
    [
      field("id", "string", ["web", "api"]),
      field("title", "string", ["Web", "API"]),
      field("subtitle", "string", ["frontend", ""]),
      field("mainstat", "number", [0.5, 0.25]),
      field("secondarystat", "number", [10, 20]),
      field("arc__ok", "number", [0.75, 1]),
      field("arc__err", "number", [0.25, 0]),
      field("detail__Owner", "string", ["team-a", null]),
      field("color", "string", ["#ff0000", ""]),
    ],
    { name: "nodes" }
  )
  const edgeFrame = frame(
    [
      field("id", "string", ["e1", "e2"]),
      field("source", "string", ["web", "web"]),
      field("target", "string", ["api", "cache"]),
      field("detail__Calls", "number", [7, 9]),
    ],
    { name: "edges" }
  )

  it("reads node titles, stats, details and colors", () => {
    const { nodes } = build([nodeFrame, edgeFrame])
    const web = nodes[0]
    expect(web.title).toBe("Web")
    expect(web.subtitle).toBe("frontend")
    expect(web.mainStat?.display.text).toBe("0.5")
    expect(web.mainStat?.name).toBe("Main stat")
    expect(web.secondaryStat?.display.text).toBe("10")
    expect(web.details).toEqual([{ name: "Owner", text: "team-a" }])
    expect(web.color).toBe("#ff0000")
    expect(nodes[1].details).toEqual([])
    expect(nodes[1].color).toBeNull()
  })

  it("adds nodes that only appear as edge endpoints", () => {
    const { nodes } = build([nodeFrame, edgeFrame])
    expect(nodes.map((node) => node.id)).toEqual(["web", "api", "cache"])
  })

  it("tells the frames apart by their fields, whatever their order", () => {
    const { nodes, edges } = build([edgeFrame, nodeFrame])
    expect(nodes.map((node) => node.title)).toEqual(["Web", "API", "cache"])
    expect(edges).toHaveLength(2)
  })

  it("treats a frame named nodes as nodes even when it has source and target", () => {
    const { nodes, edges } = build([
      frame(
        [
          field("id", "string", ["a"]),
          field("source", "string", ["x"]),
          field("target", "string", ["y"]),
        ],
        { name: "nodes" }
      ),
    ])
    expect(nodes.map((node) => node.id)).toEqual(["a"])
    expect(edges).toEqual([])
  })

  it("keeps arcs as fractions with a color each", () => {
    const { nodes } = build([nodeFrame, edgeFrame], {
      overrides: [{ match: { field: "arc__err" }, color: "critical" }],
    })
    expect(nodes[0].arcs.map((arc) => [arc.name, arc.fraction])).toEqual([
      ["ok", 0.75],
      ["err", 0.25],
    ])
    expect(nodes[0].arcs[1].color).toBe("var(--threshold-critical)")
    expect(nodes[0].arcs[0].color).toMatch(/^var\(--series-\d\)$/)
    expect(nodes[1].arcs.map((arc) => arc.name)).toEqual(["ok"])
  })

  it("scales arcs that add up to more than one", () => {
    const { nodes } = build([
      frame([
        field("id", "string", ["a"]),
        field("arc__x", "number", [3]),
        field("arc__y", "number", [1]),
      ]),
    ])
    expect(nodes[0].arcs.map((arc) => arc.fraction)).toEqual([0.75, 0.25])
  })

  it("reads edge details", () => {
    const { edges } = build([nodeFrame, edgeFrame])
    expect(edges[0].details).toEqual([{ name: "Calls", text: "7" }])
  })

  it("colors a highlighted node critical unless it names a color", () => {
    const { nodes } = build([
      frame([
        field("id", "string", ["a", "b"]),
        field("highlighted", "bool", [true, true]),
        field("color", "string", ["", "#00ff00"]),
      ]),
    ])
    expect(nodes.map((node) => node.color)).toEqual([
      "var(--threshold-critical)",
      "#00ff00",
    ])
  })

  it("ignores a color that is not plain color syntax", () => {
    const { nodes } = build([
      frame([
        field("id", "string", ["a"]),
        field("color", "string", ["red; background: url(x)"]),
      ]),
    ])
    expect(nodes[0].color).toBeNull()
  })
})

describe("graphModel self loops", () => {
  it("drops them from the edges and counts them on the node", () => {
    const { nodes, edges } = build([
      frame([
        field("source", "string", ["a", "a", "a"]),
        field("target", "string", ["a", "a", "b"]),
      ]),
    ])
    expect(edges).toHaveLength(1)
    expect(nodes.find((node) => node.id === "a")?.loops).toBe(2)
    expect(nodes.find((node) => node.id === "b")?.loops).toBe(0)
  })
})

describe("graphModel caps", () => {
  it("keeps the nodes with the most edges when there are too many", () => {
    const sources = ["hub", "hub"]
    const targets = ["a", "b"]
    for (let i = 0; i < maxNodes + 49; i++) {
      sources.push(`leaf${i}`)
      targets.push(i % 2 === 0 ? "hub" : `leaf${i + 1}`)
    }
    const model = build([
      frame([
        field("source", "string", sources),
        field("target", "string", targets),
      ]),
    ])
    expect(model.nodes).toHaveLength(maxNodes)
    expect(model.nodes.some((node) => node.id === "hub")).toBe(true)
    const kept = new Set(model.nodes.map((node) => node.id))
    for (const edge of model.edges) {
      expect(kept.has(edge.source) && kept.has(edge.target)).toBe(true)
    }
    expect(model.note).toMatch(/^Showing 300 of \d+ services\./)
  })

  it("reports the total in the services note", () => {
    const count = maxNodes + 5
    const model = build([
      frame([
        field(
          "source",
          "string",
          Array.from({ length: count }, (_, i) => `s${i}`)
        ),
        field(
          "target",
          "string",
          Array.from({ length: count }, (_, i) => `t${i}`)
        ),
      ]),
    ])
    expect(model.note).toContain(`Showing 300 of ${count * 2} services.`)
  })

  it("keeps the first 1000 edges and says so", () => {
    const count = maxEdges + 20
    const model = build([
      frame([
        field(
          "source",
          "string",
          Array.from({ length: count }, (_, i) => `a${i % 50}`)
        ),
        field(
          "target",
          "string",
          Array.from({ length: count }, (_, i) => `b${i % 50}`)
        ),
        field(
          "id",
          "string",
          Array.from({ length: count }, (_, i) => `e${i}`)
        ),
      ]),
    ])
    expect(model.edges).toHaveLength(maxEdges)
    expect(model.edges[0].id).toBe("e0")
    expect(model.note).toBe(`Showing ${maxEdges} of ${count} connections.`)
  })

  it("has no note under the caps", () => {
    const model = build([
      frame([
        field("source", "string", ["a"]),
        field("target", "string", ["b"]),
      ]),
    ])
    expect(model.note).toBeNull()
  })
})

describe("graphModel without usable frames", () => {
  it("explains what to query when no frame has the fields", () => {
    const model = graphModel(
      [frame([field("value", "number", [1])])],
      createFieldResolver(panel())
    )
    expect(model).toEqual({ reason: missingFramesReason })
    expect(missingFramesReason).toBe(
      "A node graph needs an edges query with id, source and target columns, or a nodes query with an id column."
    )
  })

  it("explains it for no frames at all", () => {
    expect(graphModel([], createFieldResolver(panel())).reason).toBe(
      missingFramesReason
    )
  })
})

describe("detail rows", () => {
  it("lists a node's stats, arcs, details and self calls in that order", () => {
    const { nodes } = build([
      frame(
        [
          field("source", "string", ["a", "a"]),
          field("target", "string", ["a", "b"]),
        ],
        { name: "edges" }
      ),
      frame(
        [
          field("id", "string", ["a"]),
          field("mainstat", "number", [5]),
          field("arc__ok", "number", [0.5]),
          field("detail__Owner", "string", ["team"]),
        ],
        { name: "nodes" }
      ),
    ])
    expect(nodeRows(nodes[0])).toEqual([
      { name: "Main stat", text: "5" },
      { name: "ok", text: "50.0%" },
      { name: "Owner", text: "team" },
      { name: "Self calls", text: "1" },
    ])
  })

  it("lists an edge's stats and details", () => {
    const { edges } = build([
      frame([
        field("source", "string", ["a"]),
        field("target", "string", ["b"]),
        field("mainstat", "number", [2]),
        field("secondarystat", "number", [9]),
        field("detail__Path", "string", ["/pay"]),
      ]),
    ])
    expect(edgeRows(edges[0])).toEqual([
      { name: "Main stat", text: "2" },
      { name: "Secondary stat", text: "9" },
      { name: "Path", text: "/pay" },
    ])
  })

  it("uses a display name override as the stat label", () => {
    const { edges } = build(
      [
        frame([
          field("source", "string", ["a"]),
          field("target", "string", ["b"]),
          field("mainstat", "number", [2]),
        ]),
      ],
      { overrides: [{ match: { field: "mainstat" }, displayName: "Latency" }] }
    )
    expect(edgeRows(edges[0])[0].name).toBe("Latency")
  })
})
