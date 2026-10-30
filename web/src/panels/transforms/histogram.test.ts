import { describe, expect, it } from "vitest"

import { histogram } from "./histogram"
import type { PreparedFrame } from "./names"

const sample: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "t", type: "time", values: [1, 2, 3, 4, 5] },
    { name: "v", type: "number", values: [1, 2, 2, 3, 9] },
  ],
}

function columns(frame: PreparedFrame): unknown[][] {
  return frame.fields.map((field) => field.values)
}

describe("histogram", () => {
  it("counts values into equal buckets, the last one holding the maximum", () => {
    const [out] = histogram([sample], { type: "histogram", bucketCount: 4 })
    expect(out.fields.map((field) => field.name)).toEqual(["xMin", "xMax", "v"])
    expect(columns(out)).toEqual([
      [1, 3, 5, 7],
      [3, 5, 7, 9],
      [3, 1, 0, 1],
    ])
  })

  it("aligns buckets to the size and the offset", () => {
    const [aligned] = histogram([sample], { type: "histogram", bucketSize: 5 })
    expect(columns(aligned)).toEqual([
      [0, 5],
      [5, 10],
      [4, 1],
    ])
    const [shifted] = histogram([sample], {
      type: "histogram",
      bucketSize: 5,
      bucketOffset: 1,
    })
    expect(columns(shifted)).toEqual([
      [1, 6],
      [6, 11],
      [4, 1],
    ])
  })

  it("counts every number field against the same buckets, or all together when combined", () => {
    const second: PreparedFrame = {
      ref: "B",
      fields: [{ name: "w", type: "number", values: [4, 4] }],
    }
    const [each] = histogram([sample, second], {
      type: "histogram",
      bucketSize: 5,
    })
    expect(each.fields.map((field) => field.name)).toEqual([
      "xMin",
      "xMax",
      "v",
      "w",
    ])
    expect(columns(each).slice(2)).toEqual([
      [4, 1],
      [2, 0],
    ])
    const [combined] = histogram([sample, second], {
      type: "histogram",
      bucketSize: 5,
      combine: true,
    })
    expect(combined.fields.map((field) => field.name)).toEqual([
      "xMin",
      "xMax",
      "Count",
    ])
    expect(columns(combined)[2]).toEqual([6, 1])
  })

  it("makes one bucket for a single value", () => {
    const single: PreparedFrame = {
      ref: "A",
      fields: [{ name: "v", type: "number", values: [7, 7] }],
    }
    const [out] = histogram([single], { type: "histogram" })
    expect(columns(out)).toEqual([[7], [7], [2]])
  })

  it("leaves frames alone when there are no numbers", () => {
    const text: PreparedFrame = {
      ref: "A",
      fields: [{ name: "s", type: "string", values: ["a"] }],
    }
    expect(histogram([text], { type: "histogram" })).toEqual([text])
  })
})
