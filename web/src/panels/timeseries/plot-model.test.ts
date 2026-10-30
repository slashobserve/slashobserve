import { describe, expect, it } from "vitest"

import type { DataPanel } from "@/spec"

import type { FieldConfig } from "../field-config"
import type { AlignedSeries } from "./align"
import {
  bandBase,
  chartSeries,
  ghostsDrawn,
  hiddenCursor,
  legendCells,
  nearestSpan,
  peerCursorAt,
  plotStateFor,
  thresholdsFor,
  thresholdValueAt,
  tooltipAnchor,
  type ChartSeries,
} from "./plot-model"
import { axesInUse } from "./series-config"
import type { Cell } from "./stack"

function config(props: Partial<FieldConfig> = {}): FieldConfig {
  return {
    thresholdMode: "absolute",
    colorBy: "last",
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
    hideInLegend: false,
    hideInTooltip: false,
    align: "auto",
    filterable: false,
    inspect: false,
    hideHeader: false,
    stackGroup: "A",
    ...props,
  }
}

function aligned(key: string, values: Cell[] = [1, 2]) {
  const field = { name: key, type: "number", values } as AlignedSeries["field"]
  return { key, label: key, values, field }
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
    interpolation: "linear",
    lineWidth: 2,
    points: "auto",
    pointSize: 4,
    spanNulls: false,
    axis: "left",
    stack: "none",
    hidden: false,
    values,
    fieldValues: values,
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
    expect(state.bands).toEqual([
      { series: [2, 1], dir: -1 },
      { series: [3, 2], dir: -1 },
    ])
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
    expect(state.bands).toEqual([{ series: [3, 1], dir: -1 }])
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
    expect(state.bands).toEqual([{ series: [3, 2], dir: -1 }])
  })

  it("fills a negative stack towards zero, the other way round from a positive one", () => {
    const up = stacked("up", [5, 5])
    const down1 = stacked("down1", [-1, -1])
    const down2 = stacked("down2", [-2, -2])
    const { state } = plotStateFor([1, 2], [], [up, down1, down2], new Set())
    expect(state.bands).toEqual([{ series: [3, 2], dir: 1 }])
  })
})

describe("plotStateFor value transforms", () => {
  it("draws a negative-Y series negated and keeps its original values", () => {
    const flipped = series("a", [1, null, 3], {
      config: config({ valueTransform: "negativeY" }),
    })
    const { state } = plotStateFor([1, 2, 3], [], [flipped], new Set())
    expect(state.data[1]).toEqual([-1, null, -3])
    expect(flipped.values).toEqual([1, null, 3])
    expect(flipped.fieldValues).toEqual([1, null, 3])
  })

  it("draws a constant series as its first value", () => {
    const flat = series("a", [null, 4, 9], {
      config: config({ valueTransform: "constant" }),
    })
    const { state } = plotStateFor([1, 2, 3], [], [flat], new Set())
    expect(state.data[1]).toEqual([4, 4, 4])
  })

  it("stacks a negative-Y series below zero", () => {
    const up = series("up", [5, 5], { stack: "normal" })
    const down = series("down", [2, 2], {
      stack: "normal",
      config: config({ stack: "normal", valueTransform: "negativeY" }),
    })
    const { state } = plotStateFor([1, 2], [], [up, down], new Set())
    expect(state.data.slice(1)).toEqual([
      [5, 5],
      [-2, -2],
    ])
  })
})

describe("plotStateFor stack groups", () => {
  const inGroup = (key: string, values: Cell[], stackGroup: string) =>
    series(key, values, {
      stack: "normal",
      config: config({ stack: "normal", stackGroup }),
    })

  it("stacks only the series of one group together", () => {
    const { state, below } = plotStateFor(
      [1, 2],
      [],
      [
        inGroup("a", [1, 1], "A"),
        inGroup("b", [10, 10], "B"),
        inGroup("c", [100, 100], "A"),
      ],
      new Set()
    )
    expect(state.data.slice(1)).toEqual([
      [1, 1],
      [10, 10],
      [101, 101],
    ])
    expect(below).toEqual([null, null, 0])
    expect(state.bands).toEqual([{ series: [3, 1], dir: -1 }])
  })
})

describe("plotStateFor fill below to", () => {
  const upper = (props: Partial<FieldConfig> = {}, fill = 0) =>
    series("up", [5, 6], {
      fill,
      config: config({ fillBelowTo: "down", ...props }),
    })
  const lower = series("down", [1, 2])

  it("bands the series down to the one with that name at 20 percent", () => {
    const { state } = plotStateFor([1, 2], [], [upper(), lower], new Set())
    expect(state.bands).toEqual([
      {
        series: [1, 2],
        dir: -1,
        fill: { color: "var(--series-1)", percent: 20 },
      },
    ])
  })

  it("uses the series' own fill percent when it has one", () => {
    const { state } = plotStateFor(
      [1, 2],
      [],
      [upper({}, 35), lower],
      new Set()
    )
    expect(state.bands[0].fill?.percent).toBe(35)
  })

  it("offsets the band past the ghost lines", () => {
    const { state } = plotStateFor(
      [1, 2],
      [[7, 8]],
      [upper(), lower],
      new Set()
    )
    expect(state.bands[0].series).toEqual([2, 3])
  })

  it("ignores a name no series carries, itself and a stacked series", () => {
    const missing = upper({ fillBelowTo: "nope" })
    const itself = upper({ fillBelowTo: "up" })
    expect(
      plotStateFor([1, 2], [], [missing, lower], new Set()).state.bands
    ).toEqual([])
    expect(
      plotStateFor([1, 2], [], [itself, lower], new Set()).state.bands
    ).toEqual([])
    const stackedUpper = series("up", [5, 6], {
      stack: "normal",
      config: config({ stack: "normal", fillBelowTo: "down" }),
    })
    expect(
      plotStateFor([1, 2], [], [stackedUpper, lower], new Set()).state.bands
    ).toEqual([])
  })
})

describe("plotStateFor span nulls", () => {
  const xs = [0, 10, 20, 30]
  const gappy: Cell[] = [1, null, 3, 4]

  it("joins gaps up to the number of milliseconds", () => {
    const { state } = plotStateFor(
      xs,
      [],
      [series("a", gappy, { spanNulls: 20000 })],
      new Set()
    )
    expect(state.data[1]).toEqual([1, undefined, 3, 4])
  })

  it("leaves a longer gap and the gaps of a series without a limit alone", () => {
    const { state } = plotStateFor(
      xs,
      [],
      [
        series("a", gappy, { spanNulls: 19999 }),
        series("b", gappy, { spanNulls: true }),
        series("c", gappy),
      ],
      new Set()
    )
    expect(state.data.slice(1)).toEqual([gappy, gappy, gappy])
  })
})

describe("legendCells", () => {
  it("reduces each series' own field values, not the aligned slots", () => {
    const rows = [
      series("a", [1, 2, 3, undefined], { fieldValues: [1, 2, 3] }),
      series("b", [undefined, 10, undefined, 20], { fieldValues: [10, 20] }),
    ]
    expect(legendCells(rows, ["last", "count"])).toEqual([
      ["3", "3"],
      ["20", "2"],
    ])
  })

  it("gives the values of one column the same decimals", () => {
    const rows = [
      series("a", [2.4], { config: config({ unit: "short" }) }),
      series("b", [2.45], { config: config({ unit: "short" }) }),
    ]
    expect(legendCells(rows, ["max"])).toEqual([["2.40"], ["2.45"]])
  })

  it("has no columns without reducers", () => {
    expect(legendCells([series("a", [1])], [])).toEqual([[]])
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

describe("tooltipAnchor", () => {
  it("anchors the tooltip at the cursor of the panel under the pointer", () => {
    expect(tooltipAnchor({ idx: 4, left: 120, top: 30 }, true, false)).toEqual({
      idx: 4,
      left: 120,
      top: 30,
      peer: false,
    })
    expect(tooltipAnchor({ idx: 0, left: 0 }, true, false)).toEqual({
      idx: 0,
      left: 0,
      top: 0,
      peer: false,
    })
  })

  it("anchors a peer tooltip only when the peer tooltip is on", () => {
    expect(tooltipAnchor({ idx: 4, left: 120, top: 30 }, false, true)).toEqual({
      idx: 4,
      left: 120,
      top: 30,
      peer: true,
    })
    expect(
      tooltipAnchor({ idx: 4, left: 120, top: 30 }, false, false)
    ).toBeNull()
  })

  it("treats the pointer as the owner even when the peer tooltip is on", () => {
    expect(tooltipAnchor({ idx: 4, left: 120, top: 30 }, true, true)).toEqual({
      idx: 4,
      left: 120,
      top: 30,
      peer: false,
    })
  })

  it.each([
    ["no point under the cursor", { idx: null, left: 120, top: 30 }],
    ["an unset point", { left: 120, top: 30 }],
    ["a cursor outside the plot", { idx: 4, left: -10, top: 30 }],
    ["a missing cursor position", { idx: 4, left: null, top: null }],
  ])("shows no tooltip for %s", (_name, cursor) => {
    expect(tooltipAnchor(cursor, true, false)).toBeNull()
    expect(tooltipAnchor(cursor, false, true)).toBeNull()
  })
})

describe("peerCursorAt", () => {
  it("places a peer cursor at mid height", () => {
    expect(peerCursorAt(120, 400, 200)).toEqual({ left: 120, top: 100 })
  })

  it.each([0, 400])("keeps a cursor on the edge at %s", (left) => {
    expect(peerCursorAt(left, 400, 200)).toEqual({ left, top: 100 })
  })

  it.each([
    ["left of the plot", -1],
    ["right of the plot", 401],
    ["missing", null],
    ["not a number", NaN],
    ["infinite", Infinity],
  ])("hides a cursor that is %s", (_name, left) => {
    expect(peerCursorAt(left, 400, 200)).toEqual(hiddenCursor)
  })
})
