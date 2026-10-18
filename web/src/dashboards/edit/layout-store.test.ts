import { beforeEach, describe, expect, it } from "vitest"
import type { Layout } from "react-grid-layout"
import { collides } from "react-grid-layout/core"

import type { Dashboard } from "@/spec"

import { applyLayout, layoutFromSpec, moveItem, settle } from "./layout"
import { isDirtyNow, useLayoutStore } from "./layout-store"

type Pos = [id: string, x: number, y: number, w: number, h: number]

function specOf(...panels: Pos[]): Dashboard {
  return {
    specVersion: "0",
    title: "layout",
    time: { from: "now-1h", to: "now" },
    panels: panels.map(([id, x, y, w, h]) => ({
      id,
      type: "stat" as const,
      title: id,
      gridPos: { x, y, w, h },
      datasource: "prom",
      queries: [{ query: "up" }],
    })),
  }
}

function state() {
  return useLayoutStore.getState()
}

function session() {
  const current = state().session
  if (!current) throw new Error("no session")
  return current
}

function find(layout: Layout, id: string) {
  const found = layout.find((entry) => entry.i === id)
  if (!found) throw new Error(`no item ${id}`)
  return found
}

function pos(layout: Layout, id: string) {
  const { x, y, w, h } = find(layout, id)
  return { x, y, w, h }
}

beforeEach(() => state().end())

describe("rebase", () => {
  it("keeps the other version's change to a panel the user did not touch", () => {
    const base = specOf(
      ["a", 0, 0, 12, 4],
      ["b", 12, 0, 12, 4],
      ["c", 0, 4, 24, 4]
    )
    state().start("d1", 1, base)
    state().commitLayout(
      settle(
        session().layout.map((entry) =>
          entry.i === "c" ? { ...entry, h: 5 } : entry
        )
      ),
      "c"
    )

    state().rebase(
      2,
      specOf(["a", 0, 0, 16, 4], ["b", 16, 0, 8, 4], ["c", 0, 4, 24, 4])
    )

    const { layout, baseVersion } = session()
    expect(baseVersion).toBe(2)
    expect(pos(layout, "a")).toEqual({ x: 0, y: 0, w: 16, h: 4 })
    expect(pos(layout, "b")).toEqual({ x: 16, y: 0, w: 8, h: 4 })
    expect(pos(layout, "c")).toEqual({ x: 0, y: 4, w: 24, h: 5 })
  })

  it("settles an edit that collides with a panel the other version added", () => {
    const base = specOf(
      ["a", 0, 0, 12, 4],
      ["b", 12, 0, 6, 4],
      ["c", 0, 4, 24, 4]
    )
    state().start("d1", 1, base)
    state().commitLayout(moveItem(session().layout, "b", 2, 0), "b")

    state().rebase(
      2,
      specOf(
        ["banner", 0, 0, 24, 3],
        ["a", 0, 3, 12, 4],
        ["b", 12, 3, 6, 4],
        ["c", 0, 7, 24, 4]
      )
    )

    const { layout } = session()
    expect(layout.map((entry) => entry.i).sort()).toEqual([
      "a",
      "b",
      "banner",
      "c",
    ])
    expect(pos(layout, "banner")).toEqual({ x: 0, y: 0, w: 24, h: 3 })
    expect(find(layout, "b").x).toBe(14)
    for (const [index, first] of layout.entries()) {
      for (const second of layout.slice(index + 1)) {
        expect(collides(first, second), `${first.i} and ${second.i}`).toBe(
          false
        )
      }
    }
  })

  it("drops edits to panels the other version removed", () => {
    state().start("d1", 1, specOf(["a", 0, 0, 12, 4], ["b", 12, 0, 12, 4]))
    state().commitLayout(moveItem(session().layout, "b", -2, 0), "b")
    state().rebase(2, specOf(["a", 0, 0, 12, 4]))
    expect(session().layout.map((entry) => entry.i)).toEqual(["a"])
    expect(isDirtyNow()).toBe(false)
  })

  it("is clean when the user changed nothing", () => {
    state().start("d1", 1, specOf(["a", 0, 0, 12, 4]))
    state().rebase(2, specOf(["a", 0, 0, 16, 4]))
    expect(pos(session().layout, "a").w).toBe(16)
    expect(isDirtyNow()).toBe(false)
  })
})

describe("dirty state", () => {
  it("compares against the compacted starting layout of a Spec with gaps", () => {
    state().start("d1", 1, specOf(["a", 0, 0, 12, 4], ["b", 0, 10, 12, 4]))
    expect(pos(session().layout, "b").y).toBe(4)
    expect(isDirtyNow()).toBe(false)

    state().commitLayout(moveItem(session().layout, "b", 1, 0), "b")
    expect(isDirtyNow()).toBe(true)

    state().commitLayout(moveItem(session().layout, "b", -1, 0), "b")
    expect(isDirtyNow()).toBe(false)
  })
})

describe("announcements", () => {
  beforeEach(() => state().start("d1", 1, specOf(["a", 0, 0, 12, 4])))

  it("keeps the announcement when the grid echoes the same layout", () => {
    state().commitLayout(moveItem(session().layout, "a", 1, 0), "moved")
    state().setLayout(structuredClone(session().layout))
    expect(session().announcement).toBe("moved")
  })

  it("clears the announcement when the grid reports a mouse change", () => {
    state().commitLayout(moveItem(session().layout, "a", 1, 0), "moved")
    state().setLayout(moveItem(session().layout, "a", 3, 0))
    expect(session().announcement).toBe("")
  })

  it("counts repeated identical announcements", () => {
    state().commitLayout(session().layout, "edge")
    const first = session().announcementCount
    state().commitLayout(session().layout, "edge")
    expect(session().announcementCount).toBe(first + 1)
  })
})

describe("row children", () => {
  const withRow: Dashboard = {
    specVersion: "0",
    title: "rows",
    time: { from: "now-1h", to: "now" },
    panels: [
      {
        id: "a",
        type: "stat",
        title: "a",
        gridPos: { x: 0, y: 0, w: 12, h: 4 },
        datasource: "prom",
        queries: [{ query: "up" }],
      },
      {
        id: "r",
        type: "row",
        title: "r",
        gridPos: { x: 0, y: 4, w: 24, h: 1 },
        panels: [
          {
            id: "child",
            type: "text",
            gridPos: { x: 0, y: 5, w: 12, h: 4 },
            content: "",
          },
        ],
      },
    ],
  }

  it("lays out row children by their own gridPos and leaves the row out", () => {
    expect(layoutFromSpec(withRow).map((item) => item.i)).toEqual([
      "a",
      "child",
    ])
  })

  it("writes a moved child back into its row", () => {
    const moved = applyLayout(
      withRow,
      layoutFromSpec(withRow).map((item) =>
        item.i === "child" ? { ...item, x: 6, w: 6 } : item
      )
    )
    const row = moved.panels[1]
    expect(row.type === "row" ? row.panels?.[0].gridPos : undefined).toEqual({
      x: 6,
      y: 5,
      w: 6,
      h: 4,
    })
  })
})
