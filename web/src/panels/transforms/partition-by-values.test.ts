import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { partitionByValues } from "./partition-by-values"

const table: PreparedFrame = {
  ref: "A",
  name: "load",
  fields: [
    { name: "host", type: "string", values: ["a", "b", "a", "b"] },
    { name: "env", type: "string", values: ["p", "p", "p", "q"] },
    { name: "cpu", type: "number", values: [1, 2, 3, 4] },
  ],
}

describe("partitionByValues", () => {
  it("makes one frame per value combination in order of first appearance", () => {
    const out = partitionByValues([table], {
      type: "partitionByValues",
      fields: ["host", "env"],
    })
    expect(out.map((frame) => frame.labels)).toEqual([
      { host: "a", env: "p" },
      { host: "b", env: "p" },
      { host: "b", env: "q" },
    ])
    expect(out.map((frame) => frame.fields.map((field) => field.name))).toEqual(
      [["cpu"], ["cpu"], ["cpu"]]
    )
    expect(out[0].fields[0].values).toEqual([1, 3])
  })

  it("keeps the partition fields when asked", () => {
    const [first] = partitionByValues([table], {
      type: "partitionByValues",
      fields: ["host"],
      keepFields: true,
    })
    expect(first.fields.map((field) => field.name)).toEqual([
      "host",
      "env",
      "cpu",
    ])
    expect(first.fields[0].values).toEqual(["a", "a"])
  })

  it("names the frames from the values instead of labels", () => {
    const named = partitionByValues([table], {
      type: "partitionByValues",
      fields: ["host", "env"],
      naming: "name",
    })
    expect(named.map((frame) => frame.name)).toEqual(["a p", "b p", "b q"])
    const appended = partitionByValues([table], {
      type: "partitionByValues",
      fields: ["host"],
      naming: "append",
    })
    expect(appended.map((frame) => frame.name)).toEqual(["load a", "load b"])
    expect(appended[0].labels).toBeUndefined()
  })

  it("keeps existing labels and passes frames that lack a field through", () => {
    const labelled = { ...table, labels: { job: "x" } }
    const [first] = partitionByValues([labelled], {
      type: "partitionByValues",
      fields: ["host"],
    })
    expect(first.labels).toEqual({ job: "x", host: "a" })
    const out = partitionByValues([table], {
      type: "partitionByValues",
      fields: ["nope"],
    })
    expect(out).toEqual([table])
  })
})
