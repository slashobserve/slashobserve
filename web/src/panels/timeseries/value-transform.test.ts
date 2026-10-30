import { describe, expect, it } from "vitest"

import { transformValues } from "./value-transform"

describe("transformValues", () => {
  it("negates numbers and keeps gaps for negativeY", () => {
    expect(transformValues([1, -2, 0, null, undefined], "negativeY")).toEqual([
      -1,
      2,
      -0,
      null,
      undefined,
    ])
  })

  it("repeats the first finite value for constant", () => {
    expect(transformValues([null, Number.NaN, 4, 9], "constant")).toEqual([
      4, 4, 4, 4,
    ])
  })

  it("returns a copy unchanged for constant without a finite value and without a kind", () => {
    const empty = [null, undefined, Number.NaN]
    const same = transformValues(empty, "constant")
    expect(same).toEqual(empty)
    expect(same).not.toBe(empty)
    const plain = [1, 2]
    expect(transformValues(plain, undefined)).toEqual(plain)
    expect(transformValues(plain, undefined)).not.toBe(plain)
  })

  it("does not change its input", () => {
    const values = [1, 2]
    transformValues(values, "negativeY")
    expect(values).toEqual([1, 2])
  })
})
