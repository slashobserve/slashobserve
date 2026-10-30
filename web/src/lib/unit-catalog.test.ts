import { describe, expect, it } from "vitest"

import schemaText from "../../../spec/schema/observespec.v0.json?raw"

import { unitGroups, unitLabel } from "./unit-catalog"
import { parseUnit } from "./units"

const schema = JSON.parse(schemaText) as {
  $defs: { unit: { anyOf: { enum?: string[] }[] } }
}

const schemaIds = schema.$defs.unit.anyOf.flatMap((branch) => branch.enum ?? [])
const catalogIds = unitGroups.flatMap((group) =>
  group.units.map((unit) => unit.id)
)

describe("unit catalog", () => {
  it("lists exactly the ids the schema accepts, each once", () => {
    expect(new Set(catalogIds).size).toBe(catalogIds.length)
    expect([...catalogIds].sort()).toEqual([...schemaIds].sort())
  })

  it("parses every schema id to itself, so each has a formatting rule", () => {
    for (const id of schemaIds) {
      expect(parseUnit(id), id).toEqual({ kind: "id", id })
    }
  })

  it("labels every custom kind", () => {
    expect(unitLabel("time:YYYY-MM-DD")).toBe('Custom time "YYYY-MM-DD"')
    expect(unitLabel("si:mF")).toBe('Custom si "mF"')
    expect(unitLabel("count:req")).toBe('Custom count "req"')
    expect(unitLabel("currency:€")).toBe('Custom currency "€"')
  })

  it("labels ids and custom units", () => {
    expect(unitLabel("s")).toBe("seconds (s)")
    expect(unitLabel("none")).toBe("None")
    expect(unitLabel(undefined)).toBe("None")
    expect(unitLabel("suffix:req")).toBe('Custom suffix "req"')
    expect(unitLabel("prefix:$")).toBe('Custom prefix "$"')
    expect(unitLabel("")).toBe("None")
  })

  it("labels unrecognised ids as unknown instead of none", () => {
    expect(unitLabel("bogus")).toBe('Unknown unit "bogus"')
    expect(unitLabel("constructor")).toBe('Unknown unit "constructor"')
  })

  it("labels blank custom text by its kind", () => {
    expect(unitLabel("suffix: ")).toBe('Custom suffix " "')
    expect(unitLabel("suffix:")).toBe('Custom suffix ""')
    expect(unitLabel("prefix:")).toBe('Custom prefix ""')
  })

  it("quotes custom text so embedded quotes stay unambiguous", () => {
    expect(unitLabel('suffix:a"b')).toBe('Custom suffix "a\\"b"')
    expect(unitLabel('prefix:a"b')).toBe('Custom prefix "a\\"b"')
  })
})
