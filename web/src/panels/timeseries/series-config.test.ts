import { describe, expect, it } from "vitest"

import type { FieldConfig } from "../field-config"
import {
  axesInUse,
  axisRange,
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
    stack: "none",
    axis: "left",
    hidden: false,
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

  it("marks configured hidden series", () => {
    const [style] = seriesStyles(aligned, [config({ hidden: true })], new Map())
    expect(style.hidden).toBe(true)
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
})

describe("axisRange", () => {
  it("keeps a hard end and leaves the other automatic", () => {
    const axis = { percent: false, min: -5 }
    expect(axisRange(axis, 1, 10)).toEqual([-5, 10.5])
  })

  it("fixes a percent axis to 0..1", () => {
    expect(axisRange({ percent: true }, 0.2, 1)).toEqual([0, 1])
  })

  it("widens a collapsed range", () => {
    expect(axisRange({ percent: false, min: 3, max: 3 }, 1, 2)).toEqual([3, 4])
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
  it("falls back to the defaults for missing or unknown options", () => {
    expect(timeseriesDisplay({ legend: "left", tooltip: 4 })).toEqual({
      legend: "bottom",
      legendValues: [],
      tooltip: "single",
    })
  })

  it("keeps known reducers and drops unknown ones", () => {
    expect(
      timeseriesDisplay({
        legend: "right",
        legendValues: ["mean", "nope", "max"],
        tooltip: "hidden",
      })
    ).toEqual({
      legend: "right",
      legendValues: ["mean", "max"],
      tooltip: "hidden",
    })
  })
})
