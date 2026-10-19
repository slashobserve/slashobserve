import { describe, expect, it } from "vitest"

import schemaText from "../../../spec/schema/observespec.v0.json?raw"

import { unitGroups, unitLabel } from "./unit-catalog"
import { formatValue } from "./units"

const schema = JSON.parse(schemaText) as {
  $defs: { unit: { anyOf: [{ enum: string[] }] } }
}

const schemaIds = schema.$defs.unit.anyOf[0].enum
const catalogIds = unitGroups.flatMap((group) =>
  group.units.map((unit) => unit.id)
)

describe("unit catalog", () => {
  it("lists exactly the ids the schema accepts, each once", () => {
    expect(new Set(catalogIds).size).toBe(catalogIds.length)
    expect([...catalogIds].sort()).toEqual([...schemaIds].sort())
  })

  it("formats every listed unit", () => {
    for (const id of schemaIds) {
      expect(formatValue(1.5, id), id).not.toBe("")
    }
  })

  it("labels ids and custom units", () => {
    expect(unitLabel("s")).toBe("seconds (s)")
    expect(unitLabel("none")).toBe("None")
    expect(unitLabel(undefined)).toBe("None")
    expect(unitLabel("suffix:req")).toBe('Custom suffix "req"')
    expect(unitLabel("prefix:$")).toBe('Custom prefix "$"')
  })
})
