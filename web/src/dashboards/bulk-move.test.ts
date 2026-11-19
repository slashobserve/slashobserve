import { describe, expect, it } from "vitest"

import { bulkMoveResult, splitByFolder } from "./bulk-move"

const dashboards = [
  { id: "a", folder: { id: "f1" } },
  { id: "b", folder: { id: "f2" } },
  { id: "c", folder: { id: "f1" } },
]

describe("splitByFolder", () => {
  it("leaves out the dashboards that are already in the target", () => {
    const { moving, staying } = splitByFolder(dashboards, "f1")
    expect(moving.map((dashboard) => dashboard.id)).toEqual(["b"])
    expect(staying.map((dashboard) => dashboard.id)).toEqual(["a", "c"])
  })

  it("moves everything when none is in the target", () => {
    const { moving, staying } = splitByFolder(dashboards, "f9")
    expect(moving).toHaveLength(3)
    expect(staying).toEqual([])
  })
})

describe("bulkMoveResult", () => {
  it("says how many moved", () => {
    expect(
      bulkMoveResult({ folder: "Ops", moved: 3, already: 0, failed: [] })
    ).toEqual({ kind: "success", text: "Moved 3 dashboards to Ops" })
    expect(
      bulkMoveResult({ folder: "Ops", moved: 1, already: 0, failed: [] })
    ).toEqual({ kind: "success", text: "Moved 1 dashboard to Ops" })
  })

  it("adds the ones that were already there", () => {
    expect(
      bulkMoveResult({ folder: "Ops", moved: 2, already: 1, failed: [] })
    ).toEqual({
      kind: "success",
      text: "Moved 2 dashboards to Ops. 1 was already there.",
    })
    expect(
      bulkMoveResult({ folder: "Ops", moved: 1, already: 2, failed: [] }).text
    ).toBe("Moved 1 dashboard to Ops. 2 were already there.")
  })

  it("says nothing moved when all were already there", () => {
    expect(
      bulkMoveResult({ folder: "Ops", moved: 0, already: 2, failed: [] })
    ).toEqual({ kind: "info", text: "Both are already in Ops." })
    expect(
      bulkMoveResult({ folder: "Ops", moved: 0, already: 1, failed: [] }).text
    ).toBe("It is already in Ops.")
    expect(
      bulkMoveResult({ folder: "Ops", moved: 0, already: 3, failed: [] }).text
    ).toBe("All 3 are already in Ops.")
  })

  it("names what failed and why", () => {
    expect(
      bulkMoveResult({
        folder: "Ops",
        moved: 2,
        already: 0,
        failed: [
          { title: "Orders", reason: "You cannot edit it." },
          { title: "Fleet", reason: "It was deleted." },
        ],
      })
    ).toEqual({
      kind: "error",
      text: "Moved 2 of 4 dashboards to Ops. Could not move Orders (You cannot edit it) or Fleet (It was deleted).",
    })
  })

  it("reports a total failure without a moved count", () => {
    expect(
      bulkMoveResult({
        folder: "Ops",
        moved: 0,
        already: 0,
        failed: [{ title: "Orders", reason: "Server error." }],
      })
    ).toEqual({
      kind: "error",
      text: "Could not move Orders (Server error).",
    })
  })
})
