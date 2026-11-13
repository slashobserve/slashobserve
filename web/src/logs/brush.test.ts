import { describe, expect, it } from "vitest"

import {
  brushRange,
  bucketAt,
  bucketSpans,
  moveEdge,
  selectionOf,
  walkBucket,
} from "./brush"

const minute = 60_000
const start = Date.parse("2026-11-06T10:00:00Z")
const spans = bucketSpans(
  Array.from({ length: 10 }, (_, index) => start + index * minute),
  minute
)

describe("bucketAt", () => {
  it("maps a position to the bucket under it", () => {
    expect(bucketAt(0, 600, 60)).toBe(0)
    expect(bucketAt(95, 600, 60)).toBe(9)
    expect(bucketAt(100, 600, 60)).toBe(10)
  })

  it("clamps a position outside the strip to the first and last bucket", () => {
    expect(bucketAt(-40, 600, 60)).toBe(0)
    expect(bucketAt(600, 600, 60)).toBe(59)
    expect(bucketAt(900, 600, 60)).toBe(59)
  })

  it("gives bucket 0 for a strip with no width", () => {
    expect(bucketAt(5, 0, 60)).toBe(0)
  })
})

describe("selectionOf", () => {
  it("orders the two indexes", () => {
    expect(selectionOf(5, 2)).toEqual({ from: 2, to: 5 })
  })

  it("is null for a click", () => {
    expect(selectionOf(3, 3)).toBeNull()
  })
})

describe("brushRange", () => {
  it("runs from the first bucket's start to the last bucket's end", () => {
    expect(brushRange(spans, 2, 5)).toEqual({
      from: "2026-11-06T10:02:00.000Z",
      to: "2026-11-06T10:06:00.000Z",
    })
  })

  it("gives the same range for a drag to the left", () => {
    expect(brushRange(spans, 5, 2)).toEqual(brushRange(spans, 2, 5))
  })

  it("is null when both indexes are equal", () => {
    expect(brushRange(spans, 4, 4)).toBeNull()
  })

  it("is null when an index has no bucket", () => {
    expect(brushRange(spans, 2, 40)).toBeNull()
  })

  it("rounds the edges to whole seconds", () => {
    const uneven = [
      { start: start + 400, end: start + 1400 },
      { start: start + 1400, end: start + 2600 },
    ]
    expect(brushRange(uneven, 0, 1)).toEqual({
      from: "2026-11-06T10:00:00.000Z",
      to: "2026-11-06T10:00:03.000Z",
    })
  })
})

describe("bucketSpans", () => {
  it("ends each bucket one step after its start", () => {
    expect(bucketSpans([1000, 61_000], 60_000)).toEqual([
      { start: 1000, end: 61_000 },
      { start: 61_000, end: 121_000 },
    ])
  })
})

describe("walkBucket", () => {
  it.each([
    ["ArrowRight", null, 0],
    ["ArrowRight", 3, 4],
    ["ArrowRight", 9, 9],
    ["ArrowLeft", null, 9],
    ["ArrowLeft", 3, 2],
    ["ArrowLeft", 0, 0],
    ["Home", 5, 0],
    ["End", 5, 9],
    ["a", 5, null],
  ])("%s from %s gives %s", (key, current, expected) => {
    expect(walkBucket(key, current, 10)).toBe(expected)
  })

  it("gives nothing for no buckets", () => {
    expect(walkBucket("ArrowRight", null, 0)).toBeNull()
  })
})

describe("moveEdge", () => {
  it("moves the start edge by one bucket", () => {
    expect(moveEdge({ from: 3, to: 6 }, "from", -1, 10)).toEqual({
      from: 2,
      to: 6,
    })
  })

  it("keeps the edges one bucket apart", () => {
    expect(moveEdge({ from: 5, to: 6 }, "from", 1, 10)).toEqual({
      from: 5,
      to: 6,
    })
    expect(moveEdge({ from: 5, to: 6 }, "to", -1, 10)).toEqual({
      from: 5,
      to: 6,
    })
  })

  it("stops at the ends of the strip", () => {
    expect(moveEdge({ from: 0, to: 4 }, "from", -1, 10)).toEqual({
      from: 0,
      to: 4,
    })
    expect(moveEdge({ from: 4, to: 9 }, "to", 1, 10)).toEqual({
      from: 4,
      to: 9,
    })
  })
})
