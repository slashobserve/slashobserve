import { describe, expect, it } from "vitest"

import { windowRows } from "../traces/trace-model"
import { estimatedRowHeight, logWindow, rowOffsets } from "./log-window"

const keys = (count: number) =>
  Array.from({ length: count }, (_, i) => `row-${i}`)

describe("rowOffsets", () => {
  it("adds the estimate for rows not measured yet", () => {
    expect(rowOffsets(keys(3), new Map())).toEqual([0, 28, 56, 84])
  })

  it("shifts the rows after a tall measured row", () => {
    expect(rowOffsets(keys(3), new Map([["row-1", 100]]))).toEqual([
      0, 28, 128, 156,
    ])
  })
})

describe("logWindow", () => {
  it.each([0, 27, 28, 500, 2_000, 5_000])(
    "matches the fixed-height window at scroll %i",
    (scrollTop) => {
      const offsets = rowOffsets(keys(300), new Map())
      expect(
        logWindow({ offsets, scrollTop, viewport: 400, overscan: 10 })
      ).toEqual(
        windowRows({
          count: 300,
          rowHeight: estimatedRowHeight,
          scrollTop,
          viewport: 400,
          openIndex: -1,
          detailHeight: 0,
          overscan: 10,
        })
      )
    }
  )

  it("places the rows after a tall row by their measured offsets", () => {
    const offsets = rowOffsets(keys(300), new Map([["row-0", 1_000]]))
    expect(
      logWindow({ offsets, scrollTop: 1_000, viewport: 56, overscan: 0 })
    ).toEqual({ start: 1, end: 4, before: 1_000, after: 296 * 28 })
  })

  it("clamps a scroll past the end", () => {
    const offsets = rowOffsets(keys(10), new Map())
    expect(
      logWindow({ offsets, scrollTop: 10_000, viewport: 400, overscan: 0 })
    ).toEqual({ start: 10, end: 10, before: 280, after: 0 })
  })

  it("widens by the overscan and clamps at both ends", () => {
    const offsets = rowOffsets(keys(20), new Map())
    expect(
      logWindow({ offsets, scrollTop: 0, viewport: 56, overscan: 5 })
    ).toEqual({ start: 0, end: 8, before: 0, after: 12 * 28 })
    expect(
      logWindow({ offsets, scrollTop: 504, viewport: 56, overscan: 5 })
    ).toEqual({ start: 13, end: 20, before: 13 * 28, after: 0 })
  })

  it("gives an empty window for no rows", () => {
    expect(
      logWindow({ offsets: [0], scrollTop: 0, viewport: 400, overscan: 10 })
    ).toEqual({ start: 0, end: 0, before: 0, after: 0 })
  })
})
