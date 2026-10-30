import { describe, expect, it } from "vitest"

import { groupingToMatrix } from "./grouping-to-matrix"
import type { PreparedFrame } from "./names"

const table: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "host", type: "string", values: ["h1", "h1", "h2"] },
    { name: "service", type: "string", values: ["web", "db", "web"] },
    { name: "count", type: "number", values: [1, 2, 3] },
  ],
}

function columns(frame: PreparedFrame): unknown[][] {
  return frame.fields.map((field) => field.values)
}

describe("groupingToMatrix", () => {
  it("turns the first three fields into a matrix of column, row and value", () => {
    const [out] = groupingToMatrix([table], { type: "groupingToMatrix" })
    expect(out.fields.map((field) => field.name)).toEqual([
      "service\\host",
      "h1",
      "h2",
    ])
    expect(columns(out)).toEqual([
      ["web", "db"],
      [1, 2],
      [3, null],
    ])
    expect(out.fields.map((field) => field.type)).toEqual([
      "string",
      "number",
      "number",
    ])
  })

  it("takes the fields it is told to", () => {
    const [out] = groupingToMatrix([table], {
      type: "groupingToMatrix",
      column: "service",
      row: "host",
      value: "count",
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "host\\service",
      "web",
      "db",
    ])
    expect(columns(out)).toEqual([
      ["h1", "h2"],
      [1, 3],
      [2, null],
    ])
  })

  it.each([
    ["zero", 0, "number"],
    ["empty", "", "other"],
    ["true", true, "other"],
    ["false", false, "other"],
  ] as const)("fills empty cells for %s", (empty, filled, type) => {
    const [out] = groupingToMatrix([table], { type: "groupingToMatrix", empty })
    expect(out.fields[2].values).toEqual([3, filled])
    expect(out.fields[2].type).toBe(type)
  })

  it("leaves a frame alone when a named field is missing or it has too few fields", () => {
    const missing = groupingToMatrix([table], {
      type: "groupingToMatrix",
      row: "nothing",
    })
    expect(missing[0]).toEqual(table)
    const narrow: PreparedFrame = { ref: "A", fields: table.fields.slice(0, 2) }
    expect(groupingToMatrix([narrow], { type: "groupingToMatrix" })[0]).toEqual(
      narrow
    )
  })
})
