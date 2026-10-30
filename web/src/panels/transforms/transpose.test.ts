import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { transpose } from "./transpose"

const table: PreparedFrame = {
  ref: "A",
  name: "stats",
  fields: [
    { name: "x", type: "number", values: [1, 2] },
    { name: "y", type: "number", values: [3, null] },
  ],
}

describe("transpose", () => {
  it("turns fields into rows and rows into fields", () => {
    const [out] = transpose([table], { type: "transpose" })
    expect(out.name).toBe("stats")
    expect(out.fields.map((field) => field.name)).toEqual([
      "Field",
      "Value 1",
      "Value 2",
    ])
    expect(out.fields.map((field) => field.values)).toEqual([
      ["x", "y"],
      [1, 3],
      [2, null],
    ])
    expect(out.fields.map((field) => field.type)).toEqual([
      "string",
      "number",
      "number",
    ])
  })

  it("names the first field and the rest", () => {
    const [out] = transpose([table], {
      type: "transpose",
      firstName: "Metric",
      restName: "Series",
    })
    expect(out.fields.map((field) => field.name)).toEqual([
      "Metric",
      "Series 1",
      "Series 2",
    ])
  })

  it.each([
    ["zero", 0, "number"],
    ["empty", "", "other"],
    ["true", true, "other"],
  ] as const)("fills empty cells for %s", (empty, filled, type) => {
    const [out] = transpose([table], { type: "transpose", empty })
    expect(out.fields[2].values).toEqual([2, filled])
    expect(out.fields[2].type).toBe(type)
  })

  it("keeps mixed columns as they are", () => {
    const mixed: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "name", type: "string", values: ["a"] },
        { name: "n", type: "number", values: [5] },
      ],
    }
    const [out] = transpose([mixed], { type: "transpose" })
    expect(out.fields[1].values).toEqual(["a", 5])
    expect(out.fields[1].type).toBe("other")
  })
})
