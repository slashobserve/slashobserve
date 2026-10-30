import { describe, expect, it } from "vitest"

import { thresholdBands, thresholdParts } from "./threshold-style"

const steps = [
  { color: "ok" },
  { value: 70, color: "warning" },
  { value: 90, color: "critical" },
]

describe("thresholdParts", () => {
  it("splits each style into lines, area and series coloring", () => {
    expect(thresholdParts("off")).toEqual({
      lines: null,
      area: false,
      series: false,
    })
    expect(thresholdParts("line")).toMatchObject({ lines: "solid" })
    expect(thresholdParts("dashed")).toMatchObject({ lines: "dashed" })
    expect(thresholdParts("area")).toMatchObject({ lines: null, area: true })
    expect(thresholdParts("line+area")).toMatchObject({
      lines: "solid",
      area: true,
    })
    expect(thresholdParts("dashed+area")).toMatchObject({
      lines: "dashed",
      area: true,
    })
    expect(thresholdParts("series")).toMatchObject({
      lines: null,
      area: false,
      series: true,
    })
  })
})

describe("thresholdBands", () => {
  it("spans from the base step up to infinity in value order", () => {
    const bands = thresholdBands(steps, "absolute", 0, 100)
    expect(bands.map(({ from, to }) => [from, to])).toEqual([
      [-Infinity, 70],
      [70, 90],
      [90, Infinity],
    ])
    expect(bands.map(({ threshold }) => threshold.color)).toEqual([
      "ok",
      "warning",
      "critical",
    ])
  })

  it("sorts steps written out of order", () => {
    const bands = thresholdBands(
      [steps[2], steps[0], steps[1]],
      "absolute",
      0,
      100
    )
    expect(bands.map(({ threshold }) => threshold.color)).toEqual([
      "ok",
      "warning",
      "critical",
    ])
  })

  it("reads percentage steps against the axis range", () => {
    const bands = thresholdBands(
      [{ color: "ok" }, { value: 50, color: "critical" }],
      "percentage",
      100,
      200
    )
    expect(bands.map(({ from, to }) => [from, to])).toEqual([
      [-Infinity, 150],
      [150, Infinity],
    ])
  })
})
