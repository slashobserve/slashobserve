import { describe, expect, it } from "vitest"

import { groupToNestedTable } from "./group-to-nested-table"
import type { PreparedFrame } from "./names"

const table: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "host", type: "string", values: ["a", "b", "a"] },
    { name: "env", type: "string", values: ["p", "q", "r"] },
    { name: "lat", type: "number", values: [10, 20, 40] },
  ],
}

describe("groupToNestedTable", () => {
  it("adds a nested frame of the remaining fields to each group row", () => {
    const [out] = groupToNestedTable([table], {
      type: "groupToNestedTable",
      by: ["host"],
      aggregate: [{ field: "lat", reducers: ["mean"] }],
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "host",
      "lat (mean)",
      "Nested frames",
    ])
    expect(out.fields[0].values).toEqual(["a", "b"])
    expect(out.fields[1].values).toEqual([25, 20])
    const nested = out.fields[2]
    expect(nested.type).toBe("other")
    expect(nested.nested).toEqual({ headers: true })
    const [first, second] = nested.values as PreparedFrame[]
    expect(first.fields.map((field) => field.name)).toEqual(["env", "lat"])
    expect(first.fields.map((field) => field.values)).toEqual([
      ["p", "r"],
      [10, 40],
    ])
    expect(second.fields.map((field) => field.values)).toEqual([["q"], [20]])
  })

  it("hides the nested headers when asked", () => {
    const [out] = groupToNestedTable([table], {
      type: "groupToNestedTable",
      by: ["host"],
      hideHeaders: true,
    })
    expect(out.fields[1].nested).toEqual({ headers: false })
  })

  it("leaves a frame alone when it has none of the named fields", () => {
    const [out] = groupToNestedTable([table], {
      type: "groupToNestedTable",
      by: ["missing"],
    })
    expect(out).toEqual(table)
  })
})
