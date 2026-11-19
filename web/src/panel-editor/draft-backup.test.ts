import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"

import { readBackup, restoreBackup } from "./draft-backup"

const gridPos = { x: 0, y: 0, w: 6, h: 4 }

function panel(title: string): DataPanel {
  return {
    id: "p1",
    type: "timeseries",
    title,
    gridPos,
    datasource: "prom",
    queries: [{ query: "up" }],
  }
}

function store(value: unknown) {
  return {
    getItem: () => (value === null ? null : JSON.stringify(value)),
  }
}

describe("readBackup", () => {
  it("returns a stored draft that differs from its base", () => {
    const backup = { base: panel("Requests"), panel: panel("Edited") }
    expect(readBackup(store(backup), "d1", "p1")).toEqual(backup)
  })

  it("ignores nothing stored, other panels, unchanged drafts and bad JSON", () => {
    expect(readBackup(store(null), "d1", "p1")).toBeNull()
    expect(
      readBackup(store({ base: panel("A"), panel: panel("B") }), "d1", "other")
    ).toBeNull()
    expect(
      readBackup(store({ base: panel("A"), panel: panel("A") }), "d1", "p1")
    ).toBeNull()
    expect(readBackup({ getItem: () => "{not json" }, "d1", "p1")).toBeNull()
    expect(readBackup(store({ base: 1, panel: 2 }), "d1", "p1")).toBeNull()
  })
})

describe("restoreBackup", () => {
  it("lays the kept changes over the panel as it is now", () => {
    const kept = { base: panel("Requests"), panel: panel("Edited") }
    const current: DataPanel = {
      ...panel("Requests"),
      queries: [{ query: "up == 1" }],
    }
    expect(restoreBackup(kept, current)).toEqual({
      ...current,
      title: "Edited",
    })
  })
})
