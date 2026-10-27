import { describe, expect, it } from "vitest"

import type { PreparedFrame } from "../transforms"
import { seriesName } from "../transforms"
import { alignFrames } from "../timeseries/align"
import {
  framesWithX,
  noXFieldReason,
  trendSource,
  xAxisFor,
  xFieldName,
  xFieldOf,
} from "./trend-model"

function frame(
  ref: string,
  fields: {
    name: string
    type: "number" | "string" | "time"
    values: unknown[]
  }[]
): PreparedFrame {
  return { ref, fields } as PreparedFrame
}

const speed = frame("A", [
  { name: "speed", type: "number", values: [30, 10, 20] },
  { name: "power", type: "number", values: [300, 100, 200] },
])

describe("xFieldOf", () => {
  it("falls back to the first number field", () => {
    expect(xFieldOf(speed, undefined)?.name).toBe("speed")
  })

  it("finds a named field", () => {
    expect(xFieldOf(speed, "power")?.name).toBe("power")
  })

  it("finds a field by its series name", () => {
    const named = frame("A", [
      { name: "x", type: "number", values: [1] },
      {
        name: "y",
        type: "number",
        values: [1],
        seriesName: "Altitude",
      } as never,
    ])
    expect(xFieldOf(named, "Altitude")?.name).toBe("y")
  })

  it("never picks a non-number field", () => {
    const text = frame("A", [{ name: "label", type: "string", values: ["a"] }])
    expect(xFieldOf(text, "label")).toBeUndefined()
    expect(xFieldOf(text, undefined)).toBeUndefined()
  })
})

describe("xFieldName", () => {
  it("reads a non-empty string", () => {
    expect(xFieldName({ xField: "speed" })).toBe("speed")
    expect(xFieldName({ xField: "" })).toBeUndefined()
    expect(xFieldName({})).toBeUndefined()
  })
})

describe("framesWithX", () => {
  it("skips a frame without the x field", () => {
    const other = frame("B", [{ name: "time", type: "time", values: [1] }])
    const source = trendSource(undefined)
    expect(framesWithX([other, speed], source)).toEqual([speed])
    expect(framesWithX([other], source)).toEqual([])
  })

  it("has a reason for a panel with no x field", () => {
    expect(noXFieldReason).toBe("A trend needs a numeric field for its x axis.")
  })
})

describe("aligning on the x field", () => {
  it("sorts rows by x and leaves the x field out of the series", () => {
    const aligned = alignFrames([speed], seriesName, [], trendSource(undefined))
    expect(aligned.xs).toEqual([10, 20, 30])
    expect(aligned.series.map((s) => s.field.name)).toEqual(["power"])
    expect(aligned.series[0].values).toEqual([100, 200, 300])
  })

  it("joins frames on shared x values", () => {
    const second = frame("B", [
      { name: "speed", type: "number", values: [20, 40] },
      { name: "torque", type: "number", values: [5, 6] },
    ])
    const aligned = alignFrames(
      [speed, second],
      seriesName,
      [],
      trendSource("speed")
    )
    expect(aligned.xs).toEqual([10, 20, 30, 40])
    expect(aligned.series.map((s) => s.values)).toEqual([
      [100, 200, 300, undefined],
      [undefined, 5, undefined, 6],
    ])
  })
})

describe("xAxisFor", () => {
  it("formats ticks with the field config", () => {
    const axis = xAxisFor(JSON.stringify({ unit: "s", mappings: [] }))
    expect(axis?.kind).toBe("number")
    expect(axis?.kind === "number" && axis.format(90)).toBe("1.5 min")
  })

  it("has no axis without a config", () => {
    expect(xAxisFor(null)).toBeNull()
  })
})
