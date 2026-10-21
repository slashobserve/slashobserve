import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"

import type { FieldConfig } from "../field-config"
import {
  bandBase,
  chartSeries,
  ghostsDrawn,
  nearestSpan,
  plotStateFor,
  thresholdsFor,
  thresholdValueAt,
  type ChartSeries,
} from "./plot-model"
import { axesInUse } from "./series-config"
import type { Cell } from "./stack"

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

function aligned(key: string, values: Cell[] = [1, 2]) {
  return { key, label: key, values }
}

function series(
  key: string,
  values: Cell[],
  props: Partial<ChartSeries> = {}
): ChartSeries {
  return {
    key,
    label: key,
    color: "var(--series-1)",
    line: "solid",
    style: "line",
    fill: 0,
    axis: "left",
    stack: "none",
    hidden: false,
    values,
    config: config(),
    ...props,
  }
}

describe("chartSeries", () => {
  it("drops series hidden by config before assigning palette slots", () => {
    const drawn = chartSeries(
      [aligned("a"), aligned("b"), aligned("c")],
      [config({ hidden: true }), config(), config()],
      new Map()
    )
    expect(drawn.map(({ key, color }) => [key, color])).toEqual([
      ["b", "var(--series-1)"],
      ["c", "var(--series-2)"],
    ])
  })

  it("keeps the slot a series already holds", () => {
    const drawn = chartSeries(
      [aligned("a"), aligned("b")],
      [config(), config()],
      new Map([["b", 4]])
    )
    expect(drawn.map(({ color }) => color)).toEqual([
      "var(--series-1)",
      "var(--series-5)",
    ])
  })
})

describe("plotStateFor", () => {
  const stacked = (key: string, values: Cell[]) =>
    series(key, values, { stack: "normal" })
  const a = stacked("a", [1, 1])
  const b = stacked("b", [10, 10])
  const c = stacked("c", [100, 100])

  it("stacks each series on the one below it and bands them together", () => {
    const { state } = plotStateFor([1, 2], [], [a, b, c], new Set())
    expect(state.data).toEqual([
      [1, 2],
      [1, 1],
      [11, 11],
      [111, 111],
    ])
    expect(state.bands).toEqual([{ series: [2, 1] }, { series: [3, 2] }])
  })

  it("drops the series above a hidden one onto the one below it", () => {
    const {
      state,
      below,
      stacked: isStacked,
    } = plotStateFor([1, 2], [], [a, b, c], new Set(["b"]))
    expect(state.data).toEqual([
      [1, 2],
      [1, 1],
      [10, 10],
      [101, 101],
    ])
    expect(state.bands).toEqual([{ series: [3, 1] }])
    expect(below).toEqual([null, null, 0])
    expect(isStacked).toEqual([true, false, true])
  })

  it("puts ghost lines first and offsets the band indices past them", () => {
    const { state } = plotStateFor([1, 2], [[7, 8]], [a, b], new Set())
    expect(state.data).toEqual([
      [1, 2],
      [7, 8],
      [1, 1],
      [11, 11],
    ])
    expect(state.bands).toEqual([{ series: [3, 2] }])
  })
})

describe("ghostsDrawn", () => {
  const ghosts = [[1, 2]]

  it("keeps the saved lines on a normal left axis", () => {
    expect(ghostsDrawn([series("a", [1])], ghosts)).toEqual(ghosts)
    expect(
      ghostsDrawn([series("a", [1], { stack: "normal" })], ghosts)
    ).toEqual(ghosts)
  })

  it("drops them when the left axis is percent-stacked", () => {
    expect(
      ghostsDrawn([series("a", [1], { stack: "percent" })], ghosts)
    ).toEqual([])
  })

  it("keeps them when only a right axis is percent-stacked", () => {
    expect(
      ghostsDrawn(
        [series("a", [1], { stack: "percent", axis: "right" })],
        ghosts
      )
    ).toEqual(ghosts)
  })
})

describe("thresholdsFor", () => {
  const panel: Pick<DataPanel, "thresholds" | "thresholdMode"> = {
    thresholds: [{ value: 5, color: "warning" }],
    thresholdMode: "percentage",
  }
  const own = [{ value: 1, color: "critical" }]

  function leftOf(rows: ChartSeries[]) {
    return axesInUse(
      rows,
      rows.map(({ config }) => config),
      {},
      0
    ).left
  }

  it("takes thresholds and mode from the first left-axis series", () => {
    const rows = [
      series("r", [1], {
        axis: "right",
        config: config({
          axis: "right",
          thresholds: [{ value: 9, color: "x" }],
        }),
      }),
      series("l", [1], {
        config: config({ thresholds: own, thresholdMode: "absolute" }),
      }),
    ]
    expect(thresholdsFor(rows, leftOf(rows), panel)).toEqual({
      list: own,
      mode: "absolute",
    })
  })

  it("falls back to the panel when no series is on the left axis", () => {
    const rows = [
      series("r", [1], {
        axis: "right",
        config: config({ axis: "right", thresholds: own }),
      }),
    ]
    const left = axesInUse(rows, [rows[0].config], {}, 1).left
    expect(thresholdsFor(rows, left, panel)).toEqual({
      list: panel.thresholds,
      mode: "percentage",
    })
  })

  it("draws none on a percent-stacked axis", () => {
    const rows = [
      series("l", [1], {
        stack: "percent",
        config: config({ stack: "percent", thresholds: own }),
      }),
    ]
    expect(thresholdsFor(rows, leftOf(rows), panel)).toBeNull()
  })

  it("draws none without a left axis or without thresholds", () => {
    expect(thresholdsFor([], undefined, panel)).toBeNull()
    const rows = [series("l", [1])]
    expect(thresholdsFor(rows, leftOf(rows), {})).toBeNull()
  })
})

describe("thresholdValueAt", () => {
  it("places a percentage threshold inside the axis range", () => {
    expect(thresholdValueAt(50, "percentage", 10, 30)).toBe(20)
    expect(thresholdValueAt(0, "percentage", 10, 30)).toBe(10)
    expect(thresholdValueAt(100, "percentage", -5, 5)).toBe(5)
  })

  it("keeps an absolute threshold where it is", () => {
    expect(thresholdValueAt(50, "absolute", 10, 30)).toBe(50)
  })
})

describe("bandBase", () => {
  it("rests a stacked series on the one below it, or on zero without one", () => {
    expect(bandBase(11, true, { drawn: 1 })).toBe(1)
    expect(bandBase(11, true, { drawn: null })).toBe(0)
    expect(bandBase(11, true, null)).toBe(0)
  })

  it("collapses an unstacked series to its own value", () => {
    expect(bandBase(7, false, null)).toBe(7)
  })
})

describe("nearestSpan", () => {
  it("picks the span the cursor is inside", () => {
    expect(
      nearestSpan(
        [
          [10, 20],
          [60, 40],
        ],
        50
      )
    ).toBe(1)
  })

  it("picks the closest span when the cursor is outside all of them", () => {
    expect(
      nearestSpan(
        [
          [10, 20],
          [90, 70],
        ],
        30
      )
    ).toBe(0)
    expect(
      nearestSpan(
        [
          [10, 20],
          [90, 70],
        ],
        64
      )
    ).toBe(1)
  })

  it("keeps the first of equally near spans", () => {
    expect(
      nearestSpan(
        [
          [0, 10],
          [20, 30],
        ],
        15
      )
    ).toBe(0)
  })
})
