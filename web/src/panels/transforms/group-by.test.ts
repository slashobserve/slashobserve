import { describe, expect, it } from "vitest"

import { groupBy } from "./group-by"
import type { PreparedFrame } from "./names"

const table: PreparedFrame = {
  ref: "A",
  name: "latency",
  labels: { job: "api" },
  fields: [
    { name: "host", type: "string", values: ["a", "b", "a", "b", "a"] },
    { name: "env", type: "string", values: ["p", "p", "p", "q", "p"] },
    { name: "lat", type: "number", values: [10, 20, 40, 50, 70] },
  ],
}

function names(frame: PreparedFrame): string[] {
  return frame.fields.map((field) => field.name)
}

describe("groupBy", () => {
  it("makes one row per group in order of first appearance and reduces the aggregated fields", () => {
    const [out] = groupBy([table], {
      type: "groupBy",
      by: ["host"],
      aggregate: [{ field: "lat", reducers: ["mean", "max", "count"] }],
    })
    expect(out.labels).toBeUndefined()
    expect(names(out)).toEqual([
      "host",
      "lat (mean)",
      "lat (max)",
      "lat (count)",
    ])
    expect(out.fields.map((field) => field.values)).toEqual([
      ["a", "b"],
      [40, 35],
      [70, 50],
      [3, 2],
    ])
    expect(out.fields.map((field) => field.type)).toEqual([
      "string",
      "number",
      "number",
      "number",
    ])
  })

  it("groups by several fields together", () => {
    const [out] = groupBy([table], {
      type: "groupBy",
      by: ["host", "env"],
      aggregate: [{ field: "lat", reducers: ["sum"] }],
    })
    expect(out.fields.map((field) => field.values)).toEqual([
      ["a", "b", "b"],
      ["p", "p", "q"],
      [120, 20, 50],
    ])
  })

  it("lists the distinct values when nothing is aggregated", () => {
    const [out] = groupBy([table], { type: "groupBy", by: ["host"] })
    expect(out.fields.map((field) => field.values)).toEqual([["a", "b"]])
  })

  it("reduces the whole frame to one row without a group field", () => {
    const [out] = groupBy([table], {
      type: "groupBy",
      aggregate: [{ field: "lat", reducers: ["sum"] }],
    })
    expect(out.fields.map((field) => field.values)).toEqual([[190]])
  })

  it("keeps only the fields it was asked for, so none when it was asked for none", () => {
    const [out] = groupBy([table], { type: "groupBy" })
    expect(out.fields).toEqual([])
  })

  it("leaves a frame alone when it has none of the named fields", () => {
    const [out] = groupBy([table], {
      type: "groupBy",
      by: ["missing"],
      aggregate: [{ field: "nothing", reducers: ["sum"] }],
    })
    expect(out).toEqual(table)
    expect(out).not.toBe(table)
  })

  it("groups each frame on its own", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [
        { name: "host", type: "string", values: ["x", "x"] },
        { name: "lat", type: "number", values: [1, 2] },
      ],
    }
    const out = groupBy([table, other], {
      type: "groupBy",
      by: ["host"],
      aggregate: [{ field: "lat", reducers: ["sum"] }],
    })
    expect(out.map((frame) => frame.ref)).toEqual(["A", "B"])
    expect(out[1].fields.map((field) => field.values)).toEqual([["x"], [3]])
  })

  it("does not change its input", () => {
    const before = structuredClone(table)
    groupBy([table], {
      type: "groupBy",
      by: ["host"],
      aggregate: [{ field: "lat", reducers: ["sum"] }],
    })
    expect(table).toEqual(before)
  })
})
