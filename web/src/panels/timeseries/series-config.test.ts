import { describe, expect, it } from "vitest"

import { displayWithDefaults } from "@/spec"

import type { FieldConfig } from "../field-config"
import {
  axesInUse,
  axisRange,
  fillPercent,
  fillStyle,
  firstStackMode,
  formatTogether,
  seriesStyles,
  timeseriesDisplay,
} from "./series-config"

function config(props: Partial<FieldConfig> = {}): FieldConfig {
  return {
    thresholdMode: "absolute",
    mappings: [],
    links: [],
    style: "line",
    fill: 0,
    lineStyle: "solid",
    lineInterpolation: "linear",
    lineWidth: 2,
    points: "auto",
    pointSize: 4,
    spanNulls: false,
    stack: "none",
    axis: "left",
    hidden: false,
    scale: "linear",
    logBase: 10,
    axisCenteredZero: false,
    axisBorder: false,
    axisColor: "text",
    ...props,
  }
}

const aligned = [{ key: "a", label: "a" }]

describe("seriesStyles", () => {
  it("uses the sticky palette slot when the config sets no color", () => {
    const [style] = seriesStyles(aligned, [config()], new Map([["a", 2]]))
    expect(style.color).toBe("var(--series-3)")
  })

  it("lets a semantic config color win over the slot", () => {
    const [style] = seriesStyles(
      aligned,
      [config({ color: "critical" })],
      new Map([["a", 2]])
    )
    expect(style.color).toBe("var(--threshold-critical)")
  })

  it("keeps a hex config color as given", () => {
    const [style] = seriesStyles(
      aligned,
      [config({ color: "#336699" })],
      new Map()
    )
    expect(style.color).toBe("#336699")
  })

  it("lets an explicit line style win over the slot dash", () => {
    const slots = new Map([["a", 9]])
    const [slotDash] = seriesStyles(aligned, [config()], slots)
    expect(slotDash).toMatchObject({ dash: [6, 4], line: "dashed" })
    const [explicit] = seriesStyles(
      aligned,
      [config({ lineStyle: "dotted" })],
      slots
    )
    expect(explicit).toMatchObject({ dash: [2, 3], line: "dotted" })
  })

  it("carries the line shape props of the config", () => {
    const [style] = seriesStyles(
      aligned,
      [
        config({
          lineInterpolation: "smooth",
          lineWidth: 5,
          points: "always",
          pointSize: 9,
          spanNulls: 60000,
        }),
      ],
      new Map()
    )
    expect(style).toMatchObject({
      interpolation: "smooth",
      lineWidth: 5,
      points: "always",
      pointSize: 9,
      spanNulls: 60000,
    })
  })

  it.each(["normal", "percent"] as const)(
    "draws a %s stacked series linear whatever its interpolation",
    (stack) => {
      const [style] = seriesStyles(
        aligned,
        [config({ lineInterpolation: "stepAfter", stack })],
        new Map()
      )
      expect(style.interpolation).toBe("linear")
    }
  )

  it.each(["smooth", "stepBefore", "stepAfter"] as const)(
    "keeps %s on an unstacked series",
    (lineInterpolation) => {
      const [style] = seriesStyles(
        aligned,
        [config({ lineInterpolation })],
        new Map()
      )
      expect(style.interpolation).toBe(lineInterpolation)
    }
  )

  it("marks configured hidden series", () => {
    const [style] = seriesStyles(aligned, [config({ hidden: true })], new Map())
    expect(style.hidden).toBe(true)
  })
})

describe("fillPercent", () => {
  it("draws bars at full opacity when the fill is 0", () => {
    expect(fillPercent("bars", 0)).toBe(100)
    expect(fillPercent("bars", 40)).toBe(40)
  })

  it("leaves a line unfilled at 0 and fills it at the given opacity", () => {
    expect(fillPercent("line", 0)).toBe(0)
    expect(fillPercent("line", 30)).toBe(30)
  })

  it("never fills points", () => {
    expect(fillPercent("points", 60)).toBe(0)
  })

  it("mixes the stroke with transparent at that opacity", () => {
    expect(fillStyle("red", 30)).toBe(
      "color-mix(in oklab, red 30%, transparent)"
    )
  })
})

describe("axesInUse", () => {
  const panel = { unit: "bytes" }

  it("takes axis settings from the first series on each axis", () => {
    const series = [
      { axis: "left", stack: "none" },
      { axis: "left", stack: "none" },
      { axis: "right", stack: "none" },
    ] as const
    const axes = axesInUse(
      series,
      [
        config({ unit: "reqps", min: 0 }),
        config({ unit: "ms" }),
        config({ unit: "s", max: 5 }),
      ],
      panel,
      0
    )
    expect(axes.left).toMatchObject({ unit: "reqps", min: 0 })
    expect(axes.right).toMatchObject({ unit: "s", max: 5 })
    expect(axes.hidden).toBeUndefined()
  })

  it("turns an axis with a percent stack into a percentunit axis", () => {
    const axes = axesInUse(
      [{ axis: "left", stack: "percent" }],
      [config({ unit: "reqps", max: 10 })],
      panel,
      0
    )
    expect(axes.left).toMatchObject({ unit: "percentunit", percent: true })
    expect(axes.left?.max).toBeUndefined()
  })

  it("puts ghost lines on the left axis with the panel settings", () => {
    expect(axesInUse([], [], panel, 2).left).toMatchObject({ unit: "bytes" })
  })

  it("takes the axis options of the first series on each axis", () => {
    const axes = axesInUse(
      [
        { axis: "left", stack: "none" },
        { axis: "left", stack: "none" },
      ],
      [
        config({
          axisLabel: "Requests",
          axisSoftMax: 10,
          scale: "log",
          logBase: 2,
          axisCenteredZero: true,
          axisWidth: 80,
          axisGrid: false,
          axisBorder: true,
          axisColor: "series",
        }),
        config({ axisLabel: "Ignored" }),
      ],
      panel,
      0
    )
    expect(axes.left).toMatchObject({
      label: "Requests",
      softMax: 10,
      scale: "log",
      logBase: 2,
      centeredZero: true,
      width: 80,
      grid: false,
      border: true,
      color: "series",
    })
  })
})

describe("axisRange", () => {
  it("keeps a hard end and leaves the other automatic", () => {
    const axis = { percent: false, min: -5 }
    expect(axisRange(axis, 1, 10)).toEqual([-5, 10.5])
  })

  it("fits a tight axis to the data instead of starting at zero", () => {
    expect(axisRange({ percent: false, tight: true }, 100, 110)).toEqual([
      99.5, 110.5,
    ])
    expect(
      axisRange({ percent: false, tight: true, min: 0 }, 100, 110)
    ).toEqual([0, 110.5])
  })

  it("fixes a percent axis to 0..1", () => {
    expect(axisRange({ percent: true }, 0.2, 1)).toEqual([0, 1])
  })

  it("lets a percent axis reach minus one when a negative group is drawn", () => {
    expect(axisRange({ percent: true }, -1, 1)).toEqual([-1, 1])
    expect(axisRange({ percent: true }, -1, 0)).toEqual([-1, 0])
    expect(axisRange({ percent: true }, -0.25, 0.75)).toEqual([-1, 1])
  })

  it("has no range for an axis none of whose series is drawn", () => {
    expect(axisRange({ percent: false }, null, null)).toEqual([null, null])
    expect(axisRange({ percent: false, min: 0, max: 10 }, null, null)).toEqual([
      null,
      null,
    ])
    expect(axisRange({ percent: true }, null, null)).toEqual([null, null])
  })

  it("widens a collapsed range", () => {
    expect(axisRange({ percent: false, min: 3, max: 3 }, 1, 2)).toEqual([3, 4])
  })

  it("widens the automatic range to soft limits and never cuts data", () => {
    expect(
      axisRange({ percent: false, softMin: -10, softMax: 100 }, 1, 10)
    ).toEqual([-10, 100])
    expect(axisRange({ percent: false, softMax: 5 }, 1, 10)).toEqual([0, 10.5])
  })

  it("centers the range on zero", () => {
    const [lo, hi] = axisRange({ percent: false, centeredZero: true }, -2, 10)
    expect(hi).toBeCloseTo(10.6)
    expect(lo).toBe(-hi!)
  })

  it("centers after the soft limits widen the range", () => {
    expect(
      axisRange({ percent: false, centeredZero: true, softMin: -50 }, -2, 10)
    ).toEqual([-50, 50])
  })

  it("lets hard limits win over soft limits and centering", () => {
    expect(
      axisRange(
        { percent: false, centeredZero: true, softMax: 100, min: 0, max: 20 },
        -2,
        10
      )
    ).toEqual([0, 20])
  })

  it("draws a log axis between powers of its base", () => {
    expect(axisRange({ percent: false, scale: "log" }, 3, 450)).toEqual([
      1, 1000,
    ])
    expect(
      axisRange({ percent: false, scale: "log", logBase: 2 }, 3, 450)
    ).toEqual([2, 512])
  })

  it("draws one decade on a log axis without positive data", () => {
    expect(axisRange({ percent: false, scale: "log" }, -5, 0)).toEqual([1, 10])
  })

  it("keeps only positive soft and hard limits on a log axis", () => {
    expect(
      axisRange(
        { percent: false, scale: "log", softMax: 1e5, softMin: -1 },
        3,
        450
      )
    ).toEqual([1, 1e5])
    expect(
      axisRange({ percent: false, scale: "log", min: 0, max: -3 }, 3, 450)
    ).toEqual([1, 1000])
    expect(
      axisRange({ percent: false, scale: "log", min: 0.5, max: 2000 }, 3, 450)
    ).toEqual([0.5, 2000])
  })

  it("ignores the scale on a percent axis", () => {
    expect(axisRange({ percent: true, scale: "log" }, 0.2, 1)).toEqual([0, 1])
  })

  it("keeps the linear range for a symlog axis", () => {
    expect(axisRange({ percent: false, scale: "symlog" }, 1, 10)).toEqual([
      0, 10.5,
    ])
  })
})

describe("firstStackMode", () => {
  it("reports the first stacked mode in use", () => {
    expect(
      firstStackMode([
        { stack: "none" },
        { stack: "percent" },
        { stack: "normal" },
      ])
    ).toBe("percent")
    expect(firstStackMode([{ stack: "none" }])).toBe("none")
  })
})

describe("formatTogether", () => {
  it("gives values of one unit the same decimals", () => {
    expect(
      formatTogether([
        { value: 2.4, config: config({ unit: "short" }) },
        { value: 2.45, config: config({ unit: "short" }) },
      ])
    ).toEqual(["2.40", "2.45"])
  })

  it("shares decimals only within a unit", () => {
    const texts = formatTogether([
      { value: 2.45, config: config({ unit: "short" }) },
      { value: 3, config: config({ unit: "bytes" }) },
    ])
    expect(texts[0]).toBe("2.45")
    expect(texts[1]).not.toContain(".")
  })

  it("shows a missing value as the placeholder", () => {
    expect(formatTogether([{ value: null, config: config() }])).toEqual(["-"])
  })
})

describe("timeseriesDisplay", () => {
  it("reads the Spec defaults merged by displayWithDefaults", () => {
    expect(timeseriesDisplay(displayWithDefaults("timeseries", {}))).toEqual({
      legend: "bottom",
      legendValues: [],
      legendSortDesc: false,
      legendHideZero: false,
      legendHideEmpty: false,
      tooltip: "single",
      tooltipSort: "desc",
      tooltipHideZeros: false,
      tooltipCumulative: false,
      xAxis: "shown",
    })
  })

  it("passes configured options through", () => {
    expect(
      timeseriesDisplay(
        displayWithDefaults("timeseries", {
          legend: "right",
          legendValues: ["mean", "max"],
          legendMode: "list",
          legendSortBy: "name",
          legendSortDesc: true,
          legendWidth: 300,
          legendHideZero: true,
          legendHideEmpty: true,
          tooltip: "hidden",
          tooltipSort: "none",
          tooltipHideZeros: true,
          tooltipCumulative: true,
          tooltipMaxWidth: 300,
          tooltipMaxHeight: 200,
          xAxis: "hidden",
        })
      )
    ).toEqual({
      legend: "right",
      legendValues: ["mean", "max"],
      legendMode: "list",
      legendSortBy: "name",
      legendSortDesc: true,
      legendWidth: 300,
      legendHideZero: true,
      legendHideEmpty: true,
      tooltip: "hidden",
      tooltipSort: "none",
      tooltipHideZeros: true,
      tooltipCumulative: true,
      tooltipMaxWidth: 300,
      tooltipMaxHeight: 200,
      xAxis: "hidden",
    })
  })
})
