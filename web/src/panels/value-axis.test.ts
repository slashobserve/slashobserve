import { describe, expect, it } from "vitest"

import {
  linearScale,
  logTicks,
  scalePosition,
  symlogTicks,
  valueAxisSettings,
  valueRange,
  type ValueAxisSettings,
} from "./value-axis"

const config = {
  axis: "left",
  axisBorder: false,
  axisColor: "text",
  scale: "linear",
  logBase: 10,
  axisCenteredZero: false,
} as const

function settings(over: Partial<ValueAxisSettings> = {}): ValueAxisSettings {
  return { ...valueAxisSettings(config), ...over }
}

describe("valueAxisSettings", () => {
  it("maps the axis side and draws the grid unless it is switched off", () => {
    expect(valueAxisSettings({ ...config, axis: "right" }).side).toBe("end")
    expect(valueAxisSettings({ ...config, axis: "hidden" }).side).toBe("hidden")
    expect(valueAxisSettings(config).grid).toBe(true)
    expect(valueAxisSettings({ ...config, axisGrid: false }).grid).toBe(false)
  })

  it("colors the axis by series only for the series color mode", () => {
    expect(valueAxisSettings({ ...config, axisColor: "series" })).toMatchObject(
      { seriesColored: true }
    )
    expect(valueAxisSettings(config).seriesColored).toBe(false)
  })
})

describe("valueRange", () => {
  it("widens to the soft limits and never cuts data", () => {
    expect(
      valueRange([2, 8], 2, settings({ softMin: 0, softMax: 10 }))
    ).toEqual([0, 10])
    expect(
      valueRange([-5, 20], 1, settings({ softMin: 0, softMax: 10 }))
    ).toEqual([-5, 20])
  })

  it("centers the range on zero from the larger end", () => {
    expect(valueRange([-2, 10], 1, settings({ centeredZero: true }))).toEqual([
      -10, 10,
    ])
  })

  it("encloses positive data in powers of the log base", () => {
    expect(valueRange([0, 900], 3, settings({ scale: "log" }))).toEqual([
      1, 1000,
    ])
    expect(
      valueRange([0, 900], 3, settings({ scale: "log", logBase: 2 }))
    ).toEqual([2, 1024])
  })

  it("falls back to one decade when a log axis has no positive data", () => {
    expect(valueRange([-3, 0], Infinity, settings({ scale: "log" }))).toEqual([
      1, 10,
    ])
  })
})

describe("scalePosition", () => {
  it("places a linear value by its share of the range", () => {
    expect(scalePosition(25, 0, 100, linearScale)).toBe(0.25)
  })

  it("places log values by decades and clamps values at or below zero", () => {
    const log = { ...linearScale, kind: "log" } as const
    expect(scalePosition(10, 1, 1000, log)).toBeCloseTo(1 / 3)
    expect(scalePosition(1000, 1, 1000, log)).toBeCloseTo(1)
    expect(scalePosition(0, 1, 1000, log)).toBe(0)
    expect(scalePosition(-4, 1, 1000, log)).toBe(0)
  })

  it("places symlog values symmetrically around zero", () => {
    const symlog = {
      ...linearScale,
      kind: "symlog",
      linearThreshold: 10,
    } as const
    expect(scalePosition(0, -1000, 1000, symlog)).toBeCloseTo(0.5)
    expect(scalePosition(100, -1000, 1000, symlog)).toBeCloseTo(
      1 - scalePosition(-100, -1000, 1000, symlog)
    )
    expect(scalePosition(1000, -1000, 1000, symlog)).toBeCloseTo(1)
  })

  it("keeps an empty range at the start", () => {
    expect(scalePosition(5, 5, 5, linearScale)).toBe(0)
  })
})

describe("logTicks", () => {
  it("lists the powers inside the range", () => {
    expect(logTicks(1, 1000, 10, 10)).toEqual([1, 10, 100, 1000])
    expect(logTicks(0.01, 1, 10, 10)).toEqual([0.01, 0.1, 1])
    expect(logTicks(2, 16, 2, 10)).toEqual([2, 4, 8, 16])
  })

  it("thins the powers when they do not fit", () => {
    expect(logTicks(1, 1e8, 10, 3)).toEqual([1, 1e3, 1e6])
  })

  it("gives no ticks for a range that is not positive", () => {
    expect(logTicks(0, 10, 10, 5)).toEqual([])
  })
})

describe("symlogTicks", () => {
  it("lists zero and the powers of ten above the threshold on both sides", () => {
    expect(symlogTicks(-1000, 1000, 1, 20)).toEqual([
      -1000, -100, -10, -1, 0, 1, 10, 100, 1000,
    ])
  })

  it("keeps only the ticks inside the range", () => {
    expect(symlogTicks(-5, 500, 1, 20)).toEqual([-1, 0, 1, 10, 100])
  })
})
