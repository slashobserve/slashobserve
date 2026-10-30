import { describe, expect, it } from "vitest"

import { configFromData } from "./config-from-data"
import type { PreparedFrame } from "./names"

const data: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "Time", type: "time", values: [1, 2] },
    { name: "cpu", type: "number", values: [1, 2] },
    { name: "host", type: "string", values: ["a", "b"] },
  ],
}
const limits: PreparedFrame = {
  ref: "B",
  fields: [
    { name: "limit", type: "number", values: [50, 80, null] },
    { name: "unit", type: "string", values: ["bytes", "percent", null] },
  ],
}

describe("configFromData", () => {
  it("drops the config frame and sets config on number fields", () => {
    const out = configFromData([data, limits], {
      type: "configFromData",
      configQuery: "B",
      mappings: [
        { field: "limit", to: "max" },
        { field: "unit", to: "unit" },
      ],
    })
    expect(out).toHaveLength(1)
    expect(out[0].fields.map((field) => field.config)).toEqual([
      undefined,
      { max: 80, unit: "percent" },
      undefined,
    ])
  })

  it("honours the reducer and the applyTo match", () => {
    const [out] = configFromData([data, limits], {
      type: "configFromData",
      configQuery: "B",
      mappings: [{ field: "limit", to: "min", reducer: "first" }],
      applyTo: { field: "host" },
    })
    expect(out.fields.map((field) => field.config)).toEqual([
      undefined,
      undefined,
      { min: 50 },
    ])
  })

  it("passes everything through when the config query has no frame", () => {
    const out = configFromData([data], {
      type: "configFromData",
      configQuery: "Z",
      mappings: [{ field: "limit", to: "max" }],
    })
    expect(out).toEqual([data])
  })

  it("does not change its input", () => {
    const frozen = Object.freeze({ ...data })
    configFromData([frozen, limits], {
      type: "configFromData",
      configQuery: "B",
      mappings: [{ field: "limit", to: "max" }],
    })
    expect(data.fields[1].config).toBeUndefined()
  })
})
