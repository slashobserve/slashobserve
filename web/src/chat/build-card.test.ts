import { describe, expect, it } from "vitest"

import type { Dashboard } from "@/spec"

import { buildRows, latestThumbnails, shownRows } from "./build-card"

function stat(id: string) {
  return {
    id,
    type: "stat" as const,
    title: id.toUpperCase(),
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    datasource: "prom",
    queries: [{ query: "up" }],
  }
}

function row(id: string, children: string[]) {
  return {
    id,
    type: "row" as const,
    title: `Row ${id}`,
    gridPos: { x: 0, y: 0, w: 24, h: 1 },
    panels: children.map(stat),
  }
}

function dashboard(panels: Dashboard["panels"]): Dashboard {
  return {
    specVersion: "0",
    title: "Checkout",
    time: { from: "now-1h", to: "now" },
    panels,
  }
}

describe("buildRows", () => {
  it("groups children under their row and skips rows with no children", () => {
    const rows = buildRows(
      dashboard([row("a", ["a1", "a2"]), row("empty", []), row("b", ["b1"])])
    )
    expect(rows.map((r) => [r.title, r.panels.map((p) => p.id)])).toEqual([
      ["Row a", ["a1", "a2"]],
      ["Row b", ["b1"]],
    ])
  })

  it("gathers consecutive panels outside any row into one untitled group", () => {
    const rows = buildRows(
      dashboard([stat("x"), stat("y"), row("a", ["a1"]), stat("z")])
    )
    expect(rows.map((r) => [r.title, r.panels.map((p) => p.id)])).toEqual([
      [null, ["x", "y"]],
      ["Row a", ["a1"]],
      [null, ["z"]],
    ])
  })

  it("has no rows for an empty build", () => {
    expect(buildRows(dashboard([]))).toEqual([])
  })
})

describe("shownRows", () => {
  const rows = (count: number) =>
    Array.from({ length: count }, (_, i) => ({
      id: `r${i}`,
      title: null,
      panels: [],
    }))

  it.each([
    [0, 0, 0],
    [3, 3, 0],
    [4, 4, 0],
    [5, 4, 1],
    [9, 4, 5],
  ])("with %i rows shows %i and hides %i", (count, shown, hidden) => {
    const result = shownRows(rows(count))
    expect(result.shown).toHaveLength(shown)
    expect(result.hidden).toBe(hidden)
  })

  it("shows the last rows in order and counts the earlier ones", () => {
    const result = shownRows(rows(6))
    expect(result.shown.map((r) => r.id)).toEqual(["r2", "r3", "r4", "r5"])
    expect(result.hidden).toBe(2)
  })
})

describe("latestThumbnails", () => {
  const at = (id: string, x: number, y: number, w: number) => ({
    ...stat(id),
    gridPos: { x, y, w, h: 4 },
  })

  it("keeps all panels of a row that fits, in grid order", () => {
    const panels = [at("b", 12, 0, 12), at("a", 0, 0, 12)]
    expect(latestThumbnails(panels).map((p) => p.id)).toEqual(["a", "b"])
  })

  it("keeps the newest panels that fit, in grid order", () => {
    const panels = [
      at("p1", 0, 0, 12),
      at("p2", 12, 0, 12),
      at("p3", 0, 4, 12),
      at("p4", 12, 4, 6),
    ]
    expect(latestThumbnails(panels).map((p) => p.id)).toEqual(["p3", "p4"])
  })

  it("keeps nothing for no panels", () => {
    expect(latestThumbnails([])).toEqual([])
  })
})
