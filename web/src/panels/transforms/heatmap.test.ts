import { describe, expect, it } from "vitest"

import { heatmap } from "./heatmap"
import type { PreparedFrame } from "./names"

const minute = 60_000

const series: PreparedFrame = {
  ref: "A",
  fields: [
    {
      name: "t",
      type: "time",
      values: [0, minute, 2 * minute, 3 * minute],
    },
    { name: "v", type: "number", values: [1, 5, 9, 17] },
  ],
}

function columns(frame: PreparedFrame): unknown[][] {
  return frame.fields.map((field) => field.values)
}

describe("heatmap", () => {
  it("counts each sample into its time column and value row, one output row per non-empty cell", () => {
    const out = heatmap([series], {
      type: "heatmap",
      yBuckets: { mode: "count", value: 4 },
    })
    expect(out).toHaveLength(1)
    expect(out[0].fields.map((field) => field.name)).toEqual([
      "xMin",
      "yMin",
      "yMax",
      "count",
    ])
    expect(out[0].fields.map((field) => field.type)).toEqual([
      "time",
      "number",
      "number",
      "number",
    ])
    expect(columns(out[0])).toEqual([
      [0, minute, 2 * minute, 3 * minute],
      [1, 5, 9, 13],
      [5, 9, 13, 17],
      [1, 1, 1, 1],
    ])
  })

  it("groups time into the requested number of columns and drops empty cells", () => {
    const [out] = heatmap([series], {
      type: "heatmap",
      xBuckets: { mode: "count", value: 2 },
      yBuckets: { mode: "count", value: 2 },
    })
    expect(columns(out)).toEqual([
      [0, 90_000],
      [1, 9],
      [9, 17],
      [2, 2],
    ])
  })

  it("groups time into columns of a size in seconds", () => {
    const [out] = heatmap([series], {
      type: "heatmap",
      xBuckets: { mode: "size", value: 120 },
      yBuckets: { mode: "count", value: 1 },
    })
    expect(columns(out)).toEqual([
      [0, 2 * minute],
      [1, 1],
      [17, 17],
      [2, 2],
    ])
  })

  it("spaces rows by the logarithm of the value on a log scale", () => {
    const wide: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "t", type: "time", values: [0, 1, 2, 3] },
        { name: "v", type: "number", values: [1, 3, 5, 8] },
      ],
    }
    const [out] = heatmap([wide], {
      type: "heatmap",
      yBuckets: { mode: "count", value: 3 },
      yScale: "log",
    })
    const [xMin, yMin, yMax, count] = columns(out)
    expect(xMin).toEqual([0, 1, 2, 3])
    expect(count).toEqual([1, 1, 1, 1])
    ;[1, 2, 4, 4].forEach((edge, at) => expect(yMin[at]).toBeCloseTo(edge))
    ;[2, 4, 8, 8].forEach((edge, at) => expect(yMax[at]).toBeCloseTo(edge))
  })

  it("spaces time columns by the logarithm of the time on a log scale", () => {
    const stamped: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "t", type: "time", values: [1000, 1500, 3000, 6000, 16000] },
        { name: "v", type: "number", values: [1, 1, 1, 1, 1] },
      ],
    }
    const [out] = heatmap([stamped], {
      type: "heatmap",
      xBuckets: { mode: "count", value: 4 },
      yBuckets: { mode: "count", value: 1 },
      xScale: "log",
    })
    const [xMin, , , count] = columns(out)
    expect(xMin).toEqual([1000, 2000, 4000, 8000])
    expect(count).toEqual([2, 1, 1, 1])
  })

  it("uses the x log base for the time columns", () => {
    const stamped: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "t", type: "time", values: [10, 20, 100, 200, 1000] },
        { name: "v", type: "number", values: [1, 1, 1, 1, 1] },
      ],
    }
    const [out] = heatmap([stamped], {
      type: "heatmap",
      xBuckets: { mode: "count", value: 3 },
      yBuckets: { mode: "count", value: 1 },
      xScale: "log",
      xLogBase: 10,
    })
    const [xMin, , , count] = columns(out)
    expect(xMin).toEqual([10, 46, 215])
    expect(count).toEqual([2, 2, 1])
  })

  it("spaces time columns on a symlog scale from zero", () => {
    const stamped: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "t", type: "time", values: [0, 2, 4, 10, 15] },
        { name: "v", type: "number", values: [1, 1, 1, 1, 1] },
      ],
    }
    const [out] = heatmap([stamped], {
      type: "heatmap",
      xBuckets: { mode: "count", value: 4 },
      yBuckets: { mode: "count", value: 1 },
      xScale: "symlog",
    })
    const [xMin, , , count] = columns(out)
    expect(xMin).toEqual([0, 1, 3, 7])
    expect(count).toEqual([1, 1, 1, 2])
  })

  it("gives each distinct value its own row in order of first appearance on an ordinal scale", () => {
    const states: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "t", type: "time", values: [0, 1, 2, 3] },
        { name: "v", type: "number", values: [5, 3, 5, 9] },
      ],
    }
    const [out] = heatmap([states], { type: "heatmap", yScale: "ordinal" })
    expect(columns(out)).toEqual([
      [0, 2, 1, 3],
      [5, 5, 3, 9],
      [5, 5, 3, 9],
      [1, 1, 1, 1],
    ])
  })

  it("adds up the samples of every series that land in the same cell", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [
        { name: "t", type: "time", values: [0, minute] },
        { name: "w", type: "number", values: [1, 2] },
      ],
    }
    const [out] = heatmap([series, other], {
      type: "heatmap",
      yBuckets: { mode: "count", value: 2 },
    })
    expect(columns(out)).toEqual([
      [0, minute, 2 * minute, 3 * minute],
      [1, 1, 9, 9],
      [9, 9, 17, 17],
      [2, 2, 1, 1],
    ])
  })

  it("keeps the original frames before the heatmap frame when asked", () => {
    const options = {
      type: "heatmap" as const,
      yBuckets: { mode: "count" as const, value: 4 },
    }
    expect(heatmap([series], options)).toHaveLength(1)
    const kept = heatmap([series], { ...options, keepOriginalData: true })
    expect(kept).toHaveLength(2)
    expect(kept[0]).toEqual(series)
    expect(kept[0]).not.toBe(series)
    expect(kept[1].fields.map((field) => field.name)).toEqual([
      "xMin",
      "yMin",
      "yMax",
      "count",
    ])
  })

  it("leaves frames without a time field and a number field unchanged", () => {
    const table: PreparedFrame = {
      ref: "A",
      fields: [{ name: "host", type: "string", values: ["a"] }],
    }
    expect(heatmap([table], { type: "heatmap" })).toEqual([table])
    expect(heatmap([], { type: "heatmap" })).toEqual([])
  })

  it("does not change its input", () => {
    const before = JSON.stringify(series)
    heatmap([series], { type: "heatmap", keepOriginalData: true })
    expect(JSON.stringify(series)).toBe(before)
  })
})
