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

  it("judges the edge of the window by the nearest sample outside it, across unmatched slots", () => {
    expect(isolatedIndices([5, undefined, undefined, 6], 3, 3)).toEqual([])
    expect(
      isolatedIndices([5, undefined, 6, undefined, undefined], 0, 0)
    ).toEqual([])
    expect(
      isolatedIndices([null, undefined, 6, undefined, null], 2, 2)
    ).toEqual([2])
    expect(isolatedIndices([1, 2, null, 3], 1, 1)).toEqual([])
    expect(isolatedIndices([1, 2, null, 3], 3, 3)).toEqual([3])
  })

  it("does not read the series far outside the window", () => {
    const reads = new Set<number>()
    const values = new Proxy(new Array<number | null>(100000).fill(1), {
      get(target, key, receiver) {
        if (typeof key === "string" && /^\d+$/.test(key)) reads.add(Number(key))
        return Reflect.get(target, key, receiver)
      },
    })
    expect(isolatedIndices(values, 50000, 50002)).toEqual([])
    expect(Math.max(...reads) - Math.min(...reads)).toBeLessThan(10)
  })

  it("returns nothing for empty data", () => {
    expect(isolatedIndices([], 0, 0)).toEqual([])
    expect(isolatedIndices([null, null], 0, 1)).toEqual([])
  })
})
