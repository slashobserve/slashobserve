import { describe, expect, it } from "vitest"

import { spanShortGaps } from "./gaps"

const times = [0, 10, 20, 30, 40, 50]

describe("spanShortGaps", () => {
  it("joins a gap shorter than the limit", () => {
    expect(spanShortGaps(times, [1, null, 3, 4, 5, 6], 30_000)).toEqual([
      1,
      undefined,
      3,
      4,
      5,
      6,
    ])
  })

  it("joins a gap exactly as long as the limit", () => {
    expect(spanShortGaps(times, [1, null, 3, 4, 5, 6], 20_000)[1]).toBe(
      undefined
    )
  })

  it("keeps a gap longer than the limit", () => {
    expect(spanShortGaps(times, [1, null, null, 4, 5, 6], 20_000)).toEqual([
      1,
      null,
      null,
      4,
      5,
      6,
    ])
  })

  it("keeps a gap at the start and at the end", () => {
    const values = [null, null, 3, 4, null, null]
    expect(spanShortGaps(times, values, 600_000)).toEqual(values)
  })

  it("counts missing samples inside a null run as part of the run", () => {
    const values = [1, undefined, null, undefined, 5, 6]
    expect(spanShortGaps(times, values, 40_000)).toEqual([
      1,
      undefined,
      undefined,
      undefined,
      5,
      6,
    ])
    expect(spanShortGaps(times, values, 39_999)).toEqual(values)
  })

  it("joins the short gaps of a series and keeps its long ones", () => {
    expect(spanShortGaps(times, [1, null, 3, null, null, 6], 20_000)).toEqual([
      1,
      undefined,
      3,
      null,
      null,
      6,
    ])
  })

  it("returns the same array when nothing changes", () => {
    const values = [1, 2, null, null, null, 6]
    expect(spanShortGaps(times, values, 10_000)).toBe(values)
  })
})
