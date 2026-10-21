import { describe, expect, it } from "vitest"

import { isolatedIndices } from "./points"

describe("isolatedIndices", () => {
  it("marks samples whose neighbours are both missing", () => {
    expect(isolatedIndices([null, 5, null, 7, null], 0, 4)).toEqual([1, 3])
  })

  it("marks a lone sample at either end of the data", () => {
    expect(isolatedIndices([4, null, null], 0, 2)).toEqual([0])
    expect(isolatedIndices([null, null, 4], 0, 2)).toEqual([2])
    expect(isolatedIndices([4], 0, 0)).toEqual([0])
  })

  it("leaves connected samples alone", () => {
    expect(isolatedIndices([1, 2, 3], 0, 2)).toEqual([])
    expect(isolatedIndices([1, 2, null, 3], 0, 3)).toEqual([3])
    expect(isolatedIndices([null, 1, 2, null], 0, 3)).toEqual([])
  })

  it("follows the data as it grows from one sample to many", () => {
    expect(isolatedIndices([null, null, 1], 0, 2)).toEqual([2])
    expect(isolatedIndices([null, 1, 1], 0, 2)).toEqual([])
  })

  it("only looks inside the visible index range but judges neighbours outside it", () => {
    expect(isolatedIndices([1, 2, null, 3, null, 4, null], 2, 4)).toEqual([3])
    expect(isolatedIndices([1, 2, 3], 2, 2)).toEqual([])
  })

  it("joins samples across an unmatched slot but not across a gap", () => {
    expect(isolatedIndices([1, undefined, 2], 0, 2)).toEqual([])
    expect(isolatedIndices([null, 5, undefined, 6, null], 0, 4)).toEqual([])
    expect(isolatedIndices([null, 5, undefined, null], 0, 3)).toEqual([1])
    expect(isolatedIndices([undefined, 5, undefined], 0, 2)).toEqual([1])
  })

  it("returns nothing for empty data", () => {
    expect(isolatedIndices([], 0, 0)).toEqual([])
    expect(isolatedIndices([null, null], 0, 1)).toEqual([])
  })
})
