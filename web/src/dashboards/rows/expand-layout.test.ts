import { describe, expect, it } from "vitest"

import type { Dashboard, DataPanel, RowPanel } from "@/spec"

import { ALL } from "../search"
import { expandLayout, maxClones, type LayoutItem } from "./expand-layout"

function stat(
  id: string,
  gridPos: { x: number; y: number; w: number; h: number },
  extra: Partial<DataPanel> = {}
): DataPanel {
  return {
    id,
    title: id,
    type: "stat",
    gridPos,
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function row(
  id: string,
  y: number,
  panels: DataPanel[],
  extra: Partial<RowPanel> = {}
): RowPanel {
  return {
    id,
    type: "row",
    title: id,
    gridPos: { x: 0, y, w: 24, h: 1 },
    panels,
    ...extra,
  }
}

function dashboard(panels: Dashboard["panels"]): Dashboard {
  return {
    title: "t",
    time: { from: "now-1h", to: "now" },
    panels,
  } as Dashboard
}

function expand(
  spec: Dashboard,
  options: {
    collapsed?: Record<string, boolean>
    selections?: Record<string, string[]>
    values?: Record<string, string[]>
    editing?: boolean
  } = {}
) {
  return expandLayout({
    spec,
    collapsed: options.collapsed ?? {},
    selections: options.selections ?? {},
    values: (name) => options.values?.[name],
    editing: options.editing ?? false,
  })
}

function panels(items: LayoutItem[]) {
  return items.flatMap((item) => (item.kind === "panel" ? [item] : []))
}

function keys(items: LayoutItem[]) {
  return items.map((item) => item.key)
}

const pos = (x: number, y: number, w: number, h: number) => ({ x, y, w, h })

describe("rows", () => {
  const spec = dashboard([
    row("r1", 0, [stat("a", pos(0, 1, 12, 4)), stat("b", pos(12, 1, 12, 4))]),
    row("r2", 5, [stat("c", pos(0, 6, 24, 4))], { collapsed: true }),
    stat("d", pos(0, 6, 24, 4)),
  ])

  it("lays out an expanded row's children at their own positions", () => {
    const items = expand(spec)
    expect(keys(items)).toEqual(["r1", "a", "b", "r2", "d"])
    expect(items.find((item) => item.key === "b")?.gridPos).toEqual(
      pos(12, 1, 12, 4)
    )
    expect(items[0]).toMatchObject({
      kind: "row",
      collapsed: false,
      childCount: 2,
    })
  })

  it("hides the children of a collapsed row and counts them", () => {
    const header = expand(spec).find((item) => item.key === "r2")
    expect(header).toMatchObject({
      kind: "row",
      collapsed: true,
      childCount: 1,
    })
    expect(keys(expand(spec))).not.toContain("c")
  })

  it("takes the view state over the stored collapsed flag", () => {
    const items = expand(spec, { collapsed: { r1: true, r2: false } })
    expect(keys(items)).toEqual(["r1", "r2", "c", "d"])
  })

  it("follows the stored flag only while editing", () => {
    const items = expand(spec, {
      collapsed: { r1: true, r2: false },
      editing: true,
    })
    expect(keys(items)).toEqual(["r1", "a", "b", "r2", "d"])
  })

  it("interpolates the row title", () => {
    const titled = dashboard([row("r", 0, [], { title: "Env ${env}" })])
    expect(
      expand(titled, { selections: { env: ["a", "b"] } })[0]
    ).toMatchObject({
      title: "Env a, b",
    })
  })
})

describe("panel repeat", () => {
  const repeated = (extra: Partial<DataPanel>) =>
    dashboard([
      stat("p", pos(0, 0, 8, 4), {
        title: "Env ${env}",
        repeat: "env",
        ...extra,
      }),
      stat("after", pos(0, 4, 24, 2)),
    ])
  const env = { env: ["a", "b", "c"] }

  it("fills lines of maxPerRow clones and wraps", () => {
    const items = panels(
      expand(repeated({ maxPerRow: 2 }), { selections: env })
    ).filter((item) => item.panel.id === "p")
    expect(items.map((item) => [item.title, item.gridPos])).toEqual([
      ["Env a", pos(0, 0, 12, 4)],
      ["Env b", pos(12, 0, 12, 4)],
      ["Env c", pos(0, 4, 12, 4)],
    ])
    expect(keys(items)).toEqual(["p", "p__1", "p__2"])
    expect(items.map((item) => item.first)).toEqual([true, false, false])
  })

  it("defaults to four clones per line", () => {
    const items = panels(
      expand(repeated({}), { selections: { env: ["a", "b", "c", "d", "e"] } })
    ).filter((item) => item.panel.id === "p")
    expect(items.map((item) => item.gridPos.x)).toEqual([0, 6, 12, 18, 0])
    expect(items[4].gridPos.y).toBe(4)
  })

  it("stacks clones in the v direction at the panel's own size", () => {
    const items = panels(
      expand(repeated({ repeatDirection: "v" }), { selections: env })
    ).filter((item) => item.panel.id === "p")
    expect(items.map((item) => item.gridPos)).toEqual([
      pos(0, 0, 8, 4),
      pos(0, 4, 8, 4),
      pos(0, 8, 8, 4),
    ])
  })

  it("pushes the panels below down by the added height", () => {
    const items = expand(repeated({ maxPerRow: 2 }), { selections: env })
    expect(items.find((item) => item.key === "after")?.gridPos.y).toBe(8)
  })

  it("overrides only the repeat variable in a clone's selections", () => {
    const items = panels(
      expand(repeated({}), { selections: { env: ["a", "b"], zone: ["z"] } })
    )
    expect(items[1].selections).toEqual({ env: ["b"], zone: ["z"] })
    expect(items[0].selections).toEqual({ env: ["a"], zone: ["z"] })
    expect(items[2].selections).toEqual({ env: ["a", "b"], zone: ["z"] })
  })

  it("expands All into the loaded values", () => {
    const items = panels(
      expand(repeated({}), {
        selections: { env: [ALL] },
        values: { env: ["x", "y"] },
      })
    ).filter((item) => item.panel.id === "p")
    expect(items.map((item) => item.title)).toEqual(["Env x", "Env y"])
  })

  it("keeps one panel while All has no loaded values yet", () => {
    const items = panels(expand(repeated({}), { selections: { env: [ALL] } }))
    expect(items.map((item) => item.title)).toEqual(["Env All", "after"])
  })

  it("caps clones and adds a note", () => {
    const many = Array.from({ length: 60 }, (_, index) => `v${index}`)
    const items = expand(repeated({ maxPerRow: 24 }), {
      selections: { env: many },
    })
    expect(panels(items).filter((item) => item.panel.id === "p")).toHaveLength(
      maxClones
    )
    const note = items.find((item) => item.kind === "note")
    expect(note).toMatchObject({ text: "Showing the first 50 of 60 values." })
  })

  it("returns the originals only while editing", () => {
    const items = expand(repeated({}), { selections: env, editing: true })
    expect(keys(items)).toEqual(["p", "after"])
    expect(items[0]).toMatchObject({ gridPos: pos(0, 0, 8, 4) })
  })
})

describe("row repeat", () => {
  const spec = dashboard([
    row(
      "r",
      0,
      [
        stat("inner", pos(0, 1, 12, 3), { title: "In ${env}" }),
        stat("each", pos(12, 1, 12, 3), {
          repeat: "zone",
          title: "Zone ${zone}",
        }),
      ],
      { repeat: "env", title: "Env ${env}" }
    ),
    stat("after", pos(0, 4, 24, 2)),
  ])
  const selections = { env: ["a", "b"], zone: ["p", "q"] }

  it("clones the header and children per value in order", () => {
    const items = expand(spec, { selections })
    const headers = items.filter((item) => item.kind === "row")
    expect(headers.map((item) => [item.key, item.title])).toEqual([
      ["r", "Env a"],
      ["r__r1", "Env b"],
    ])
    const inner = panels(items).filter((item) => item.panel.id === "inner")
    expect(inner.map((item) => [item.key, item.title, item.first])).toEqual([
      ["inner", "In a", true],
      ["inner__r1", "In b", false],
    ])
  })

  it("keeps inner panel repeats within each clone and stacks the blocks", () => {
    const items = expand(spec, { selections })
    const zones = panels(items).filter((item) => item.panel.id === "each")
    expect(
      zones.map((item) => [item.key, item.gridPos.y, item.selections])
    ).toEqual([
      ["each", 1, { env: ["a"], zone: ["p"] }],
      ["each__1", 1, { env: ["a"], zone: ["q"] }],
      ["each__r1", 5, { env: ["b"], zone: ["p"] }],
      ["each__r1__1", 5, { env: ["b"], zone: ["q"] }],
    ])
    expect(items.find((item) => item.key === "r__r1")?.gridPos.y).toBe(4)
    expect(items.find((item) => item.key === "after")?.gridPos.y).toBe(8)
  })

  it("stacks collapsed clones as bare headers", () => {
    const items = expand(spec, { selections, collapsed: { r: true } })
    expect(items.map((item) => [item.key, item.gridPos.y])).toEqual([
      ["r", 0],
      ["r__r1", 1],
      ["after", 5],
    ])
  })

  it("returns the original row only while editing", () => {
    expect(keys(expand(spec, { selections, editing: true }))).toEqual([
      "r",
      "inner",
      "each",
      "after",
    ])
  })
})
