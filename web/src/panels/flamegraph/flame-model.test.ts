import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  brokenOrderReason,
  colorOf,
  flameModel,
  focusText,
  fullRange,
  isFlameKey,
  missingFieldsReason,
  nodeAt,
  packagePrefix,
  search,
  shareText,
  stepFocus,
  zoomForFocus,
  zoomTo,
  type FlameKey,
  type FlameModel,
  type FlameProfile,
} from "./flame-model"

type Row = [level: number, value: number, self: number, label: string]

function panel(): DataPanel {
  return {
    id: "p1",
    title: "Flame",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "flamegraph",
    datasource: "prom",
    queries: [{ query: "profile" }],
    unit: "ms",
  } as DataPanel
}

function frame(
  rows: Row[],
  names = ["level", "value", "self", "label"],
  extra: PreparedFrame["fields"] = []
): PreparedFrame {
  return {
    ref: "A",
    fields: [
      { name: names[0], type: "number", values: rows.map((row) => row[0]) },
      { name: names[1], type: "number", values: rows.map((row) => row[1]) },
      { name: names[2], type: "number", values: rows.map((row) => row[2]) },
      { name: names[3], type: "string", values: rows.map((row) => row[3]) },
      ...extra,
    ],
  }
}

function build(frames: PreparedFrame[]): FlameModel {
  return flameModel(frames, createFieldResolver(panel()))
}

function profileOf(rows: Row[]): FlameProfile {
  const model = build([frame(rows)])
  if (model.reason !== null) throw new Error(model.reason)
  return model
}

const sample: Row[] = [
  [0, 10, 1, "main.run"],
  [1, 6, 2, "http.serve"],
  [2, 4, 4, "net/http.read"],
  [1, 3, 3, "db.query"],
]

describe("flameModel", () => {
  it("rebuilds parents from the depth-first order", () => {
    const { nodes } = profileOf(sample)
    expect(nodes.map((node) => node.parent)).toEqual([-1, 0, 1, 0])
    expect(nodes.map((node) => node.label)).toEqual([
      "main.run",
      "http.serve",
      "net/http.read",
      "db.query",
    ])
  })

  it("puts each node after its earlier siblings and shares the root total", () => {
    const { nodes, total } = profileOf(sample)
    expect(total).toBe(10)
    expect(nodes.map((node) => node.start)).toEqual([0, 0, 0, 6])
    expect(nodes.map((node) => node.share)).toEqual([1, 0.6, 0.4, 0.3])
  })

  it("starts a second branch after the first branch's whole subtree", () => {
    const { nodes } = profileOf([
      [0, 9, 0, "root"],
      [1, 5, 0, "a"],
      [2, 5, 5, "a.leaf"],
      [1, 4, 0, "b"],
      [2, 4, 4, "b.leaf"],
    ])
    expect(nodes.map((node) => node.start)).toEqual([0, 0, 0, 5, 5])
  })

  it("matches the four fields without regard to case", () => {
    const model = build([frame(sample, ["Level", "VALUE", "Self", "Label"])])
    expect(model.reason).toBeNull()
  })

  it("uses the first frame that has all four fields", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [{ name: "value", type: "number", values: [1] }],
    }
    const model = build([other, frame(sample)])
    expect(model.reason).toBeNull()
  })

  it("reads the unit from the value field", () => {
    expect(profileOf(sample).config.unit).toBe("ms")
  })

  it("explains missing fields", () => {
    expect(build([]).reason).toBe(missingFieldsReason)
    const noSelf: PreparedFrame = {
      ref: "A",
      fields: frame(sample).fields.filter((field) => field.name !== "self"),
    }
    expect(build([noSelf]).reason).toBe(missingFieldsReason)
  })

  it("explains a level jump of more than one", () => {
    const model = build([
      frame([
        [0, 10, 0, "root"],
        [2, 5, 5, "deep"],
      ]),
    ])
    expect(model.reason).toBe(brokenOrderReason)
  })

  it("explains a first row that is not the root level", () => {
    expect(build([frame([[1, 5, 5, "orphan"]])]).reason).toBe(brokenOrderReason)
  })

  it("explains a level that is not a whole number", () => {
    expect(build([frame([[0.5, 5, 5, "odd"]])]).reason).toBe(brokenOrderReason)
  })

  it("marks a frame with valueRight and selfRight as a diff profile", () => {
    const right: PreparedFrame["fields"] = [
      { name: "valueRight", type: "number", values: sample.map(() => 1) },
      { name: "selfRight", type: "number", values: sample.map(() => 1) },
    ]
    const diff = build([frame(sample, undefined, right)])
    expect(diff.reason === null && diff.diff).toBe(true)
    expect(profileOf(sample).diff).toBe(false)
  })

  it("builds a 50,000-row model", () => {
    const rows: Row[] = [[0, 0, 0, "root"]]
    const sizes = Array.from({ length: 100 }, (_, group) =>
      group < 99 ? 499 : 49_899 - 99 * 499
    )
    sizes.forEach((size, group) => {
      rows.push([1, size, 0, `pkg${group}.group`])
      for (let leaf = 0; leaf < size; leaf++) {
        rows.push([2, 1, 1, `pkg${group}.leaf${leaf}`])
      }
    })
    const leaves = sizes.reduce((sum, size) => sum + size, 0)
    rows[0][1] = leaves
    expect(rows.length).toBe(50_000)
    const profile = profileOf(rows)
    expect(profile.nodes).toHaveLength(50_000)
    expect(profile.total).toBe(leaves)
    expect(profile.levels.map((level) => level.length)).toEqual([
      1,
      100,
      leaves,
    ])
    const last = profile.nodes[49_999]
    expect(last.start + last.value).toBe(leaves)
  })
})

describe("colors", () => {
  it("takes the package prefix before the last separator", () => {
    expect(packagePrefix("net/http.read")).toBe("net/http")
    expect(packagePrefix("a/b/c")).toBe("a/b")
    expect(packagePrefix("std::vec::push")).toBe("std::vec")
    expect(packagePrefix("main")).toBe("main")
  })

  it("gives labels of one package the same color from the eight slots", () => {
    expect(colorOf("db.query")).toBe(colorOf("db.exec"))
    const used = new Set(
      Array.from({ length: 200 }, (_, i) => colorOf(`pkg${i}.fn`))
    )
    expect(used.size).toBe(8)
    for (const color of used) expect(color).toMatch(/^var\(--series-[1-8]\)$/)
  })
})

describe("zoomTo", () => {
  it("returns the clicked node's span", () => {
    const profile = profileOf(sample)
    expect(zoomTo(profile, 3)).toEqual({ start: 6, end: 9 })
    expect(zoomTo(profile, 1)).toEqual({ start: 0, end: 6 })
  })

  it("returns the whole profile for the root or a node without value", () => {
    const profile = profileOf([
      [0, 4, 4, "root"],
      [1, 0, 0, "empty"],
    ])
    expect(zoomTo(profile, 0)).toEqual(fullRange(profile))
    expect(zoomTo(profile, 1)).toEqual(fullRange(profile))
  })
})

describe("nodeAt", () => {
  it("finds the node under a point of the visible range", () => {
    const profile = profileOf(sample)
    const full = fullRange(profile)
    expect(nodeAt(profile, full, 1, 0.2)).toBe(1)
    expect(nodeAt(profile, full, 1, 0.7)).toBe(3)
    expect(nodeAt(profile, full, 2, 0.2)).toBe(2)
    expect(nodeAt(profile, full, 2, 0.7)).toBeNull()
    expect(nodeAt(profile, full, 5, 0.2)).toBeNull()
  })

  it("follows a zoomed range", () => {
    const profile = profileOf(sample)
    const range = zoomTo(profile, 3)
    expect(nodeAt(profile, range, 0, 0.5)).toBe(0)
    expect(nodeAt(profile, range, 1, 0.5)).toBe(3)
  })
})

describe("search", () => {
  it("marks labels that contain the text without regard to case", () => {
    const profile = profileOf(sample)
    expect([...(search(profile, "HTTP") ?? [])]).toEqual([1, 2])
    expect([...(search(profile, "  db ") ?? [])]).toEqual([3])
    expect(search(profile, "nothing")?.size).toBe(0)
  })

  it("marks nothing for empty text", () => {
    expect(search(profileOf(sample), "  ")).toBeNull()
  })
})

describe("shareText", () => {
  it("shows two decimals", () => {
    expect(shareText(0.6)).toBe("60.00%")
    expect(shareText(0.00042)).toBe("0.04%")
  })
})

describe("stepFocus", () => {
  const rows: Row[] = [
    [0, 10, 0, "root"],
    [1, 4, 0, "a"],
    [2, 4, 4, "a.leaf"],
    [1, 6, 1, "b"],
    [2, 2, 2, "b.first"],
    [2, 3, 3, "b.second"],
  ]
  const profile = profileOf(rows)
  const [root, a, aLeaf, b, bFirst, bSecond] = [0, 1, 2, 3, 4, 5]

  const cases: [string, number | null, FlameKey, number][] = [
    ["nothing focused enters at the root on Down", null, "ArrowDown", root],
    ["nothing focused enters at the root on Left", null, "ArrowLeft", root],
    ["Home from a leaf goes to the root", bSecond, "Home", root],
    ["Up at the root stays", root, "ArrowUp", root],
    ["Left at the root stays", root, "ArrowLeft", root],
    ["Right at the root stays", root, "ArrowRight", root],
    ["Down from the root goes to the first child", root, "ArrowDown", a],
    ["Up from a middle node goes to its parent", b, "ArrowUp", root],
    ["Down from a middle node goes to its first child", b, "ArrowDown", bFirst],
    ["Right from a middle node goes to the next sibling", a, "ArrowRight", b],
    ["Left from a middle node goes to the previous one", b, "ArrowLeft", a],
    ["Right crosses parents along the row", aLeaf, "ArrowRight", bFirst],
    ["Left crosses parents along the row", bFirst, "ArrowLeft", aLeaf],
    ["Right at the end of a row stays", bSecond, "ArrowRight", bSecond],
    ["Left at the start of a row stays", aLeaf, "ArrowLeft", aLeaf],
    ["Down on a leaf stays", aLeaf, "ArrowDown", aLeaf],
    ["Up from a leaf goes to its parent", bSecond, "ArrowUp", b],
  ]

  it.each(cases)("%s", (_name, from, key, expected) => {
    expect(stepFocus(profile, from, key)).toBe(expected)
  })

  it("treats an index outside the profile like nothing focused", () => {
    expect(stepFocus(profile, 99, "ArrowDown")).toBe(root)
  })
})

describe("isFlameKey", () => {
  it("accepts the arrows and Home only", () => {
    expect(
      ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight", "Home"].every(
        isFlameKey
      )
    ).toBe(true)
    expect(["Enter", " ", "Escape", "a"].some(isFlameKey)).toBe(false)
  })
})

describe("focusText", () => {
  it("reads the label, total with share, self and one-based level", () => {
    const profile = profileOf(sample)
    expect(focusText(profile, 1)).toBe(
      "http.serve, total 6 ms (60.00%), self 2 ms, level 2"
    )
  })
})

describe("zoomForFocus", () => {
  const profile = profileOf([
    [0, 10, 0, "root"],
    [1, 4, 0, "a"],
    [2, 4, 4, "a.leaf"],
    [1, 6, 1, "b"],
    [2, 2, 2, "b.first"],
    [2, 3, 3, "b.second"],
    [1, 0, 0, "idle"],
  ])
  const [root, a, aLeaf, b, bFirst, bSecond, idle] = [0, 1, 2, 3, 4, 5, 6]

  const cases: [string, number | null, number, number | null][] = [
    ["keeps the full view for any frame", null, bSecond, null],
    ["keeps a zoom when the frame lies inside it", a, aLeaf, a],
    ["keeps a zoom when the frame is an ancestor that covers it", a, root, a],
    ["keeps a zoom when the frame is the zoomed one", b, b, b],
    [
      "goes back to the full view for a sibling of a top level zoom",
      a,
      b,
      null,
    ],
    ["zooms to the shared parent for a sibling frame", bFirst, bSecond, b],
    ["goes to the nearest ancestor that covers the zoom", aLeaf, bSecond, null],
    ["keeps the zoom for a frame with no width", a, idle, a],
    ["keeps the full view for a frame with no width", null, idle, null],
    [
      "goes to the full view when the zoom sits in another branch",
      bFirst,
      aLeaf,
      null,
    ],
  ]

  it.each(cases)("%s", (_name, zoomed, focused, expected) => {
    expect(zoomForFocus(profile, zoomed, focused)).toBe(expected)
  })

  it("ignores an index outside the profile", () => {
    expect(zoomForFocus(profile, a, 99)).toBe(a)
  })
})
