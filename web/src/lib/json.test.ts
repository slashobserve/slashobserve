import { describe, expect, it } from "vitest"

import { sameJson, stableJson } from "./json"

describe("stableJson", () => {
  it("sorts object keys at every depth and keeps array order", () => {
    expect(stableJson({ b: 1, a: { d: [2, 1], c: 3 } })).toBe(
      '{"a":{"c":3,"d":[2,1]},"b":1}'
    )
  })

  it("makes key order irrelevant to sameJson", () => {
    expect(sameJson({ a: 1, b: 2 }, { b: 2, a: 1 })).toBe(true)
    expect(sameJson([1, 2], [2, 1])).toBe(false)
  })
})
