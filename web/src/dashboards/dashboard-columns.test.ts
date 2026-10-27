import { describe, expect, it } from "vitest"

import { dashboardColumns } from "./dashboard-columns"

describe("dashboardColumns", () => {
  it("lists the canvas columns in order", () => {
    expect(dashboardColumns.map((c) => c.header)).toEqual([
      "Title",
      "Made by",
      "Datasources",
      "Open comments",
      "Version",
      "Updated",
    ])
  })

  it("gives every column a unique id", () => {
    const ids = dashboardColumns.map((c) => c.id)
    expect(new Set(ids).size).toBe(dashboardColumns.length)
    expect(ids).toEqual([
      "title",
      "madeBy",
      "datasources",
      "openComments",
      "version",
      "updated",
    ])
  })

  it("right-aligns only Updated", () => {
    const right = dashboardColumns.filter((c) =>
      c.meta?.headerClassName?.includes("text-right")
    )
    expect(right.map((c) => c.id)).toEqual(["updated"])
    expect(right[0].meta?.cellClassName).toContain("text-right")
  })
})
