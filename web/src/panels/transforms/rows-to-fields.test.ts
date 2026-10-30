import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { configProps, rowsToFields } from "./rows-to-fields"

const rows: PreparedFrame = {
  ref: "A",
  name: "info",
  labels: { job: "x" },
  fields: [
    { name: "Field", type: "string", values: ["cpu", "mem"] },
    { name: "Value", type: "number", values: [10, 20] },
    { name: "Unit", type: "string", values: ["percent", "bytes"] },
    { name: "Max", type: "number", values: [100, 64] },
  ],
}

describe("rowsToFields", () => {
  it("turns each row into a field of its own", () => {
    const [out] = rowsToFields([rows], { type: "rowsToFields" })
    expect(out.labels).toBeUndefined()
    expect(out.fields).toEqual([
      { name: "cpu", type: "number", values: [10] },
      { name: "mem", type: "number", values: [20] },
    ])
  })

  it("reads config from the mapped columns", () => {
    const [out] = rowsToFields([rows], {
      type: "rowsToFields",
      mappings: [
        { field: "Field", to: "name" },
        { field: "Value", to: "value" },
        { field: "Unit", to: "unit" },
        { field: "Max", to: "max" },
      ],
    })
    expect(out.fields.map((field) => field.config)).toEqual([
      { unit: "percent", max: 100 },
      { unit: "bytes", max: 64 },
    ])
  })

  it("leaves a frame without a name column alone", () => {
    const numbers: PreparedFrame = {
      ref: "A",
      fields: [{ name: "n", type: "number", values: [1] }],
    }
    expect(rowsToFields([numbers], { type: "rowsToFields" })).toEqual([numbers])
  })
})

describe("configProps", () => {
  it("keeps valid values and drops the rest", () => {
    expect(
      configProps([
        ["min", "5"],
        ["max", "x"],
        ["decimals", 2.5],
        ["unit", ""],
        ["color", "#ff0000"],
        ["displayName", "CPU"],
      ])
    ).toEqual({ min: 5, color: "#ff0000", displayName: "CPU" })
    expect(configProps([["decimals", 3]])).toEqual({ decimals: 3 })
    expect(configProps([["unit", null]])).toBeUndefined()
  })
})
