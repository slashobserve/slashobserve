import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { regression } from "./regression"

function line(): PreparedFrame {
  return {
    ref: "A",
    name: "load",
    fields: [
      { name: "x", type: "number", values: [1, 2, 3, 4, 5] },
      { name: "y", type: "number", values: [3, 5, 7, 9, 11] },
    ],
  }
}

function rounded(values: readonly unknown[]): number[] {
  return values.map((value) => Number((value as number).toFixed(6)))
}

describe("regression", () => {
  it("keeps the frame and adds a fitted frame after it", () => {
    const frame = line()
    const out = regression([frame], { type: "regression" })
    expect(out).toHaveLength(2)
    expect(out[0]).toBe(frame)
    expect(out[1].name).toBe("load")
    expect(out[1].fields.map((field) => field.name)).toEqual([
      "x",
      "y regression",
    ])
    expect(rounded(out[1].fields[0].values)).toEqual([1, 2, 3, 4, 5])
    expect(rounded(out[1].fields[1].values)).toEqual([3, 5, 7, 9, 11])
  })

  it("evaluates a line at evenly spaced points when asked for a count", () => {
    const out = regression([line()], { type: "regression", predictions: 3 })
    expect(rounded(out[1].fields[0].values)).toEqual([1, 3, 5])
    expect(rounded(out[1].fields[1].values)).toEqual([3, 7, 11])
  })

  it("fits a polynomial by least squares", () => {
    const parabola: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [1, 2, 3, 4, 5] },
        { name: "y", type: "number", values: [1, 4, 9, 16, 25] },
      ],
    }
    const out = regression([parabola], {
      type: "regression",
      model: "polynomial",
      predictions: 3,
    })
    expect(rounded(out[1].fields[1].values)).toEqual([1, 9, 25])
    const cubic: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [0, 1, 2, 3, 4, 5] },
        { name: "y", type: "number", values: [0, 1, 8, 27, 64, 125] },
      ],
    }
    const fitted = regression([cubic], {
      type: "regression",
      model: "polynomial",
      degree: 3,
    })
    expect(rounded(fitted[1].fields[1].values)).toEqual([0, 1, 8, 27, 64, 125])
  })

  it("fits a straight line through the least squares of noisy points", () => {
    const noisy: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [0, 1, 2, 3] },
        { name: "y", type: "number", values: [1, 3, 2, 6] },
      ],
    }
    const out = regression([noisy], { type: "regression" })
    expect(rounded(out[1].fields[1].values)).toEqual([0.9, 2.3, 3.7, 5.1])
  })

  it("uses the time field and the first number field by default, and rounds predicted times", () => {
    const series: PreparedFrame = {
      ref: "A",
      fields: [
        {
          name: "Time",
          type: "time",
          values: [1_700_000_000_000, 1_700_000_060_000, 1_700_000_120_000],
        },
        { name: "Value", type: "number", values: [10, 20, 30] },
      ],
    }
    const out = regression([series], { type: "regression", predictions: 4 })
    expect(out[1].fields[0].type).toBe("time")
    expect(out[1].fields[0].values).toEqual([
      1_700_000_000_000, 1_700_000_040_000, 1_700_000_080_000,
      1_700_000_120_000,
    ])
    expect(rounded(out[1].fields[1].values)).toEqual([
      10, 16.666667, 23.333333, 30,
    ])
  })

  it("names the fields it fits", () => {
    const wide: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "a", type: "number", values: [1, 2, 3] },
        { name: "b", type: "number", values: [9, 9, 9] },
        { name: "c", type: "number", values: [2, 4, 6] },
      ],
    }
    const out = regression([wide], { type: "regression", x: "a", y: "c" })
    expect(out[1].fields.map((field) => field.name)).toEqual([
      "a",
      "c regression",
    ])
    expect(rounded(out[1].fields[1].values)).toEqual([2, 4, 6])
  })

  it("skips points that are not numbers and frames it cannot fit", () => {
    const gaps: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [1, 2, null, 4] },
        { name: "y", type: "number", values: [2, 4, 6, 8] },
      ],
    }
    const out = regression([gaps], { type: "regression" })
    expect(rounded(out[1].fields[0].values)).toEqual([1, 2, 4])
    expect(rounded(out[1].fields[1].values)).toEqual([2, 4, 8])

    const one: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [1] },
        { name: "y", type: "number", values: [1] },
      ],
    }
    const same: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "x", type: "number", values: [2, 2, 2] },
        { name: "y", type: "number", values: [1, 2, 3] },
      ],
    }
    const text: PreparedFrame = {
      ref: "A",
      fields: [{ name: "s", type: "string", values: ["a"] }],
    }
    expect(regression([one, same, text], { type: "regression" })).toEqual([
      one,
      same,
      text,
    ])
  })
})
