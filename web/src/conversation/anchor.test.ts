import { describe, expect, it } from "vitest"

import casesText from "../../../spec/examples/anchor/anchor-cases.json?raw"
import {
  anchorKinds,
  describeAnchor,
  encodedBytes,
  type Anchor,
} from "./anchor"

type SharedCases = {
  valid: { name: string; anchor: Anchor; describe: string }[]
}

const cases = JSON.parse(casesText) as SharedCases

describe("shared anchor cases", () => {
  it.each(cases.valid)("describes $name", ({ anchor, describe: want }) => {
    expect(describeAnchor(anchor)).toBe(want)
  })

  it("has a valid case for every kind", () => {
    const kinds = new Set(cases.valid.map((c) => c.anchor.kind))
    expect([...kinds].sort()).toEqual([...anchorKinds].sort())
  })
})

describe("encodedBytes", () => {
  const base = { kind: "title", pos: { x: 0, y: 0 } }

  it("counts the bytes of the JSON encoding", () => {
    expect(encodedBytes(base)).toBe(JSON.stringify(base).length)
    expect(encodedBytes({ ...base, quote: "é" })).toBe(
      JSON.stringify({ ...base, quote: "x" }).length + 1
    )
  })

  it("counts the characters Go escapes at their escaped length", () => {
    const plain = encodedBytes({ ...base, quote: "a" })
    expect(encodedBytes({ ...base, quote: "<" })).toBe(plain + 5)
    expect(encodedBytes({ ...base, quote: "&" })).toBe(plain + 5)
    expect(encodedBytes({ ...base, quote: "\u2028" })).toBe(plain + 5)
  })
})
