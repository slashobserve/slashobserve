import { describe, expect, it } from "vitest"

import casesText from "../../../spec/examples/anchor/anchor-cases.json?raw"
import { anchorKinds, describeAnchor, parseAnchor } from "./anchor"

type SharedCases = {
  valid: { name: string; anchor: unknown; describe: string }[]
  invalid: { name: string; anchor: unknown; error: string }[]
}

const cases = JSON.parse(casesText) as SharedCases

describe("shared anchor cases", () => {
  it.each(cases.valid)("describes $name", ({ anchor, describe: want }) => {
    expect(describeAnchor(parseAnchor(anchor))).toBe(want)
  })

  it.each(cases.invalid)("rejects $name", ({ anchor, error }) => {
    expect(() => parseAnchor(anchor)).toThrow(new Error(error))
  })

  it("has a valid case for every kind", () => {
    const kinds = new Set(cases.valid.map((c) => parseAnchor(c.anchor).kind))
    expect([...kinds].sort()).toEqual([...anchorKinds].sort())
  })
})

describe("parseAnchor", () => {
  const base = { kind: "title", pos: { x: 0, y: 0 } }

  it("keeps only the fields the wire carries", () => {
    const parsed = parseAnchor({ ...base, shown: "x" })
    expect(parsed).toEqual({ kind: "title", pos: { x: 0, y: 0 }, shown: "x" })
    expect(Object.keys(parsed)).not.toContain("from")
  })

  it.each([
    ["not an object", "row:service=payments"],
    ["null", null],
    ["an array", []],
    ["an unknown field", { ...base, colour: "red" }],
    ["an unknown field in pos", { ...base, pos: { x: 0, y: 0, z: 1 } }],
    ["a string pos", { ...base, pos: "0.4,0.3" }],
    [
      "an unknown field in window",
      { ...base, window: { from: "a", to: "b", tz: "u" } },
    ],
    ["a bad time", { kind: "time", pos: { x: 0, y: 0 }, from: "yesterday" }],
    ["a number where a string goes", { ...base, shown: 440 }],
    [
      "a label that is not a string",
      { kind: "series", pos: { x: 0, y: 0 }, series: { a: 1 } },
    ],
    [
      "a row that is an array",
      { kind: "row", pos: { x: 0, y: 0 }, row: ["a"] },
    ],
  ])("rejects %s", (_name, value) => {
    expect(() => parseAnchor(value)).toThrow(/^anchor: /)
  })
})
