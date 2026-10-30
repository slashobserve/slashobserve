import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "./names"
import { prepareTimeSeries } from "./prepare-time-series"

const a: PreparedFrame = {
  ref: "A",
  labels: { pod: "p1" },
  fields: [
    { name: "Time", type: "time", values: [1, 2] },
    { name: "Value", type: "number", values: [10, 20] },
  ],
}
const b: PreparedFrame = {
  ref: "A",
  labels: { pod: "p2" },
  fields: [
    { name: "Time", type: "time", values: [2, 3] },
    { name: "Value", type: "number", values: [30, 40] },
  ],
}
const table: PreparedFrame = {
  ref: "B",
  fields: [{ name: "x", type: "string", values: ["k"] }],
}

describe("prepareTimeSeries", () => {
  it("joins every series on time in wide format", () => {
    const [wide, rest] = prepareTimeSeries([a, b, table], {
      type: "prepareTimeSeries",
      format: "wide",
    })
    expect(wide.fields.map((field) => field.values)).toEqual([
      [1, 2, 3],
      [10, 20, null],
      [null, 30, 40],
    ])
    expect(wide.fields.map((field) => field.seriesName)).toEqual([
      undefined,
      expect.stringContaining("p1"),
      expect.stringContaining("p2"),
    ])
    expect(rest).toEqual(table)
  })

  it("splits a frame with several values into one frame per value in many format", () => {
    const multi: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "Time", type: "time", values: [1] },
        { name: "x", type: "number", values: [5] },
        { name: "y", type: "number", values: [6] },
      ],
    }
    const out = prepareTimeSeries([multi, a], {
      type: "prepareTimeSeries",
      format: "many",
    })
    expect(out.map((frame) => frame.fields.map((field) => field.name))).toEqual(
      [
        ["Time", "x"],
        ["Time", "y"],
        ["Time", "Value"],
      ]
    )
  })

  it("stacks all series into one table with label columns in long format", () => {
    const [long] = prepareTimeSeries([a, b], {
      type: "prepareTimeSeries",
      format: "long",
    })
    expect(long.fields).toEqual([
      { name: "Time", type: "time", values: [1, 2, 2, 3] },
      { name: "pod", type: "string", values: ["p1", "p1", "p2", "p2"] },
      { name: "Value", type: "number", values: [10, 20, 30, 40] },
    ])
  })

  it("changes nothing when no frame is a time series", () => {
    expect(
      prepareTimeSeries([table], { type: "prepareTimeSeries", format: "long" })
    ).toEqual([table])
  })
})
