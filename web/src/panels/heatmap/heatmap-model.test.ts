import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  calculatedRows,
  cellBox,
  cellCenter,
  cellPaint,
  cellText,
  colorPosition,
  columnAt,
  formatCounts,
  heatmapLook,
  heatmapModel,
  insetCell,
  isHeatmapKey,
  labelFits,
  legendDescription,
  legendScale,
  stepCell,
  visibleCell,
  type HeatmapKey,
} from "./heatmap-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "h1",
    title: "Heat",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "heatmap",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function series(
  times: number[],
  values: (number | null)[],
  labels: Record<string, string> = {}
): PreparedFrame {
  return {
    ref: "A",
    labels,
    fields: [
      { name: "Time", type: "time", values: times },
      { name: "Value", type: "number", values },
    ],
  }
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  const target = panel(extra)
  return heatmapModel(
    frames,
    createFieldResolver(target),
    displayWithDefaults("heatmap", display),
    target.queries
  )
}

const bucketQuery = [{ query: "x", format: "heatmap" as const }]

describe("bucket mode", () => {
  it("sorts rows by le with +Inf last", () => {
    const model = build([
      series([1], [3], { le: "+Inf" }),
      series([1], [1], { le: "10" }),
      series([1], [2], { le: "2.5" }),
    ])
    expect(model.mode).toBe("buckets")
    expect(model.rows.map((row) => row.label)).toEqual(["2.5", "10", "+Inf"])
    expect(model.cells).toEqual([[2], [1], [3]])
  })

  it("de-accumulates cumulative buckets for the heatmap format", () => {
    const model = build(
      [
        series([1, 2], [1, 0], { le: "1" }),
        series([1, 2], [3, 0], { le: "2" }),
        series([1, 2], [6, 5], { le: "+Inf" }),
      ],
      { queries: bucketQuery }
    )
    expect(model.cells).toEqual([
      [1, 0],
      [2, 0],
      [3, 5],
    ])
    expect(model.max).toBe(5)
    expect(model.min).toBe(1)
  })

  it("never goes below zero when a higher bucket is smaller", () => {
    const model = build(
      [series([1], [4], { le: "1" }), series([1], [3], { le: "2" })],
      { queries: bucketQuery }
    )
    expect(model.cells).toEqual([[4], [0]])
  })

  it("sums series that share a bound", () => {
    const model = build([
      series([1], [1], { le: "1", pod: "a" }),
      series([1], [2], { le: "1", pod: "b" }),
    ])
    expect(model.rows).toHaveLength(1)
    expect(model.cells).toEqual([[3]])
  })

  it("labels bounds with the unit and gives each bucket a range", () => {
    const model = build(
      [
        series([1], [1], { le: "0.1" }),
        series([1], [1], { le: "0.5" }),
        series([1], [1], { le: "+Inf" }),
      ],
      { unit: "s" }
    )
    expect(model.rows.map((row) => row.label)).toEqual([
      "100 ms",
      "500 ms",
      "+Inf",
    ])
    expect(model.rows.map(({ from, to }) => [from, to])).toEqual([
      [0, 0.1],
      [0.1, 0.5],
      [0.5, Infinity],
    ])
    expect(model.rows[1].range).toBe("100 ms - 500 ms")
  })

  it("leaves a series without a bound out of bucket mode", () => {
    const model = build([
      series([1], [1], { le: "1" }),
      series([1], [2], { pod: "a" }),
    ])
    expect(model.mode).toBe("ordinal")
  })
})

describe("ordinal mode", () => {
  it("makes one row per series in frame order", () => {
    const model = build([
      series([1, 2], [1, 2], { pod: "b" }),
      series([1, 2], [3, null], { pod: "a" }),
    ])
    expect(model.mode).toBe("ordinal")
    expect(model.rows.map((row) => row.label)).toEqual([
      '{pod="b"}',
      '{pod="a"}',
    ])
    expect(model.cells).toEqual([
      [1, 2],
      [3, null],
    ])
  })

  it("skips hidden fields", () => {
    const model = build(
      [series([1], [1], { pod: "a" }), series([1], [2], { pod: "b" })],
      { overrides: [{ match: { series: '{pod="a"}' }, hidden: true }] }
    )
    expect(model.rows).toHaveLength(1)
  })
})

describe("columns", () => {
  it("joins the timestamps of every frame", () => {
    const model = build([
      series([10, 30], [1, 2], { pod: "a" }),
      series([20, 30], [3, 4], { pod: "b" }),
    ])
    expect(model.xs).toEqual([10, 20, 30])
    expect(model.cells).toEqual([
      [1, null, 2],
      [null, 3, 4],
    ])
  })
})

describe("calculated mode", () => {
  const frames = [
    series([1, 2], [0, 16], { pod: "a" }),
    series([1, 2], [8, 16], { pod: "b" }),
  ]

  it("handles more values than a function call can take as arguments", () => {
    const count = 300_000
    const times = Array.from({ length: count }, (_, i) => i)
    const values = Array.from({ length: count }, (_, i) => i % 100)
    const model = build([series(times, values)], {}, { calculate: true })
    expect(model.rows).toHaveLength(calculatedRows)
    expect(model.rows[0].from).toBe(0)
  })

  it("counts values into 16 linear rows", () => {
    const model = build(frames, {}, { calculate: true })
    expect(model.mode).toBe("calculated")
    expect(model.rows).toHaveLength(calculatedRows)
    expect(model.rows[0].from).toBe(0)
    expect(model.rows[1].from).toBe(1)
    expect(model.cells[0]).toEqual([1, 0])
    expect(model.cells[8]).toEqual([1, 0])
    expect(model.cells[15]).toEqual([0, 2])
    expect(model.max).toBe(2)
    expect(model.rows[1].label).toBe("1")
  })

  it("bins on a log scale when every value is positive", () => {
    const model = build(
      [series([1], [1], { pod: "a" }), series([1], [10000], { pod: "b" })],
      {},
      { calculate: true, yScale: "log" }
    )
    expect(model.rows[4].from).toBeCloseTo(10)
    expect(model.rows[8].from).toBeCloseTo(100)
    expect(model.cells[0]).toEqual([1])
    expect(model.cells[15]).toEqual([1])
  })

  it("falls back to a linear scale when a value is not positive", () => {
    const model = build(frames, {}, { calculate: true, yScale: "log" })
    expect(model.rows[1].from).toBe(1)
  })

  it("keeps one value per row when every value is equal", () => {
    const model = build([series([1, 2], [5, 5])], {}, { calculate: true })
    expect(model.cells[0]).toEqual([1, 1])
    expect(model.max).toBe(1)
  })

  it("has no rows without numbers", () => {
    const model = build([series([1], [null])], {}, { calculate: true })
    expect(model.rows).toEqual([])
    expect(model.max).toBe(0)
  })
})

function look(display: Record<string, unknown>, min = 1, max = 10) {
  return heatmapLook(displayWithDefaults("heatmap", display), { min, max })
}

describe("colour scale", () => {
  it("runs from 10% to 100% opacity of the first series color by default", () => {
    const plain = look({}, 1, 10)
    expect(cellPaint(plain, 0).alpha).toBeCloseTo(0.1)
    expect(cellPaint(plain, 10).alpha).toBeCloseTo(1)
    expect(cellPaint(plain, 5).alpha).toBeCloseTo(0.55)
    expect(cellPaint(plain, 5).color).toBe("var(--series-1)")
  })

  it("uses the fill color of opacity mode, mapping a semantic name to its token", () => {
    expect(cellPaint(look({ colorFill: "warning" }), 5).color).toBe(
      "var(--threshold-warning)"
    )
    expect(cellPaint(look({ colorFill: "#112233" }), 5).color).toBe("#112233")
  })

  it("bends the opacity with an exponential scale and its exponent", () => {
    const bent = look({ colorScale: "exponential" }, 0, 100)
    expect(colorPosition(bent, 25)).toBeCloseTo(0.5)
    const steeper = look(
      { colorScale: "exponential", colorExponent: 2 },
      0,
      100
    )
    expect(colorPosition(steeper, 50)).toBeCloseTo(0.25)
  })

  it("leaves a scheme linear whatever the scale says", () => {
    const scheme = look(
      { colorMode: "scheme", colorScale: "exponential" },
      0,
      100
    )
    expect(colorPosition(scheme, 25)).toBeCloseTo(0.25)
  })

  it("starts a scheme at the lowest count and opacity at zero", () => {
    expect(colorPosition(look({ colorMode: "scheme" }, 20, 100), 20)).toBe(0)
    expect(colorPosition(look({}, 20, 100), 20)).toBeCloseTo(0.2)
  })

  it("takes the color range from colorMin and colorMax and clamps outside it", () => {
    const fixed = look({ colorMin: 10, colorMax: 20 }, 1, 100)
    expect(colorPosition(fixed, 15)).toBeCloseTo(0.5)
    expect(colorPosition(fixed, 5)).toBe(0)
    expect(colorPosition(fixed, 50)).toBe(1)
  })

  it("quantizes the position into the chosen number of steps", () => {
    const stepped = look({ colorMode: "scheme", colorSteps: 4 }, 0, 100)
    const positions = [0, 24, 25, 60, 74, 75, 99, 100].map((value) =>
      colorPosition(stepped, value)
    )
    expect(positions.map((p) => Number(p.toFixed(3)))).toEqual([
      0, 0, 0.333, 0.667, 0.667, 1, 1, 1,
    ])
  })

  it("reverses the ramp after quantizing", () => {
    const reversed = look(
      { colorMode: "scheme", colorSteps: 2, colorReverse: true },
      0,
      100
    )
    expect(colorPosition(reversed, 10)).toBe(1)
    expect(colorPosition(reversed, 90)).toBe(0)
  })

  it("paints a scheme with its ramp at full opacity", () => {
    const paint = cellPaint(
      look({ colorMode: "scheme", colorScheme: "reds" }),
      10
    )
    expect(paint.alpha).toBe(1)
    expect(paint.color).toBe("var(--threshold-critical)")
  })

  it("steps the legend from the lowest to the highest count", () => {
    const steps = legendScale(look({}, 1, 5))
    expect(steps.map((step) => step.value)).toEqual([1, 2, 3, 4, 5])
    expect(steps[4].paint.alpha).toBeCloseTo(1)
  })

  it("draws no more legend swatches than the color steps", () => {
    const steps = legendScale(
      look({ colorMode: "scheme", colorSteps: 3 }, 0, 10)
    )
    expect(steps.map((step) => step.value)).toEqual([0, 5, 10])
  })

  it("labels the legend with the color range, which colorMin and colorMax set", () => {
    const format = formatCounts([0, 120])
    expect(legendDescription(look({}, 0, 120), format)).toBe(
      "Color scale from 0 to 120"
    )
    expect(
      legendDescription(look({ colorMin: 10, colorMax: 50 }, 0, 120), format)
    ).toBe("Color scale from 10 to 50")
  })
})

describe("hidden cells", () => {
  it("hides null, zero and negative cells by default", () => {
    const plain = look({})
    expect(
      [null, 0, -1, 0.5].map((value) => visibleCell(plain, value))
    ).toEqual([false, false, false, true])
  })

  it("hides cells at or below hideBelow and at or above hideAbove", () => {
    const filtered = look({ hideBelow: 2, hideAbove: 8 })
    expect(
      [1, 2, 3, 7, 8, 9].map((value) => visibleCell(filtered, value))
    ).toEqual([false, false, true, true, false, false])
  })

  it("draws zeros when hideBelow is negative", () => {
    expect(visibleCell(look({ hideBelow: -1 }), 0)).toBe(true)
  })

  it("leaves hidden cells out of the color range", () => {
    const model = build(
      [series([start, start + minute, start + 2 * minute], [1, 5, 100])],
      {},
      { hideBelow: 1, hideAbove: 100 }
    )
    expect([model.min, model.max]).toEqual([5, 5])
  })
})

describe("cell look", () => {
  it("defaults to flush square cells with no values and a legend", () => {
    const plain = look({})
    expect(plain).toMatchObject({
      gap: 0,
      radius: 0,
      showValues: "never",
      legend: true,
      mode: "opacity",
      steps: null,
    })
  })

  it("reads the gap, radius, legend and values display keys", () => {
    expect(
      look({ cellGap: 3, cellRadius: 4, legend: "hidden", showValues: "auto" })
    ).toMatchObject({ gap: 3, radius: 4, legend: false, showValues: "auto" })
  })

  it("shrinks a cell by the gap around its center, never below one pixel", () => {
    const box = { left: 10, top: 20, width: 30, height: 12 }
    expect(insetCell(box, 4)).toEqual({
      left: 12,
      top: 22,
      width: 26,
      height: 8,
    })
    expect(insetCell(box, 40)).toEqual({
      left: 24.5,
      top: 25.5,
      width: 1,
      height: 1,
    })
  })

  it("fits a value text by cell size in auto mode and always in always mode", () => {
    const box = { left: 0, top: 0, width: 30, height: 20 }
    expect(labelFits("never", box, 5)).toBe(false)
    expect(labelFits("auto", box, 20)).toBe(true)
    expect(labelFits("auto", box, 27)).toBe(false)
    expect(labelFits("auto", { ...box, height: 12 }, 5)).toBe(false)
    expect(labelFits("always", { ...box, width: 2 }, 50)).toBe(true)
  })

  it("formats cell values with the cell unit and decimals", () => {
    expect(formatCounts([1, 2], "ms", 2)(1.5)).toBe("1.50 ms")
    expect(formatCounts([1, 2], "ms")(1.5)).toContain("ms")
    expect(formatCounts([1, 2])(2)).toBe("2")
  })
})

const minute = 60_000
const start = new Date(2024, 0, 2, 10, 0, 0).getTime()
const grid = build([
  series([start, start + minute, start + 2 * minute], [1, 2, 3], { le: "1" }),
  series([start, start + minute, start + 2 * minute], [4, 5, 6], { le: "2" }),
  series([start, start + minute, start + 2 * minute], [7, 8, 9], {
    le: "+Inf",
  }),
])

describe("keyboard stepping", () => {
  it("recognises only the navigation keys", () => {
    for (const key of [
      "ArrowUp",
      "ArrowDown",
      "ArrowLeft",
      "ArrowRight",
      "Home",
      "End",
    ]) {
      expect(isHeatmapKey(key)).toBe(true)
    }
    for (const key of ["Enter", "Escape", "Tab", "a", "PageUp"]) {
      expect(isHeatmapKey(key)).toBe(false)
    }
  })

  it("moves one cell per arrow key, up meaning a higher bucket", () => {
    const cases: [HeatmapKey, { row: number; col: number }][] = [
      ["ArrowUp", { row: 2, col: 1 }],
      ["ArrowDown", { row: 0, col: 1 }],
      ["ArrowLeft", { row: 1, col: 0 }],
      ["ArrowRight", { row: 1, col: 2 }],
      ["Home", { row: 1, col: 0 }],
      ["End", { row: 1, col: 2 }],
    ]
    for (const [key, expected] of cases) {
      expect(stepCell(grid, { row: 1, col: 1 }, key)).toEqual(expected)
    }
  })

  it("stays on the edge instead of wrapping", () => {
    expect(stepCell(grid, { row: 2, col: 2 }, "ArrowUp")).toEqual({
      row: 2,
      col: 2,
    })
    expect(stepCell(grid, { row: 2, col: 2 }, "ArrowRight")).toEqual({
      row: 2,
      col: 2,
    })
    expect(stepCell(grid, { row: 0, col: 0 }, "ArrowDown")).toEqual({
      row: 0,
      col: 0,
    })
    expect(stepCell(grid, { row: 0, col: 0 }, "ArrowLeft")).toEqual({
      row: 0,
      col: 0,
    })
  })

  it("starts at the top row, first bucket, or the last bucket for End", () => {
    expect(stepCell(grid, null, "ArrowDown")).toEqual({ row: 2, col: 0 })
    expect(stepCell(grid, null, "Home")).toEqual({ row: 2, col: 0 })
    expect(stepCell(grid, null, "End")).toEqual({ row: 2, col: 2 })
  })

  it("focuses nothing when the heatmap has no cells", () => {
    const empty = build([series([1], [null])], {}, { calculate: true })
    expect(stepCell(empty, null, "Home")).toBeNull()
  })
})

describe("cell geometry", () => {
  it("places row 0 at the bottom and column 0 at the left", () => {
    expect(cellBox(grid, { row: 0, col: 0 }, 90, 60)).toEqual({
      left: 0,
      top: 40,
      width: 30,
      height: 20,
    })
    expect(cellCenter(grid, { row: 2, col: 2 }, 90, 60)).toEqual({
      x: 75,
      y: 10,
    })
  })
})

describe("cell description", () => {
  it("reads the time range, bucket range and count", () => {
    const format = formatCounts([grid.min, grid.max])
    expect(cellText(grid, { row: 1, col: 1 }, format)).toBe(
      "2024-01-02 10:01:00 to 2024-01-02 10:02:00, bucket 1 - 2, count 5"
    )
  })

  it("ends the last time bucket one step after its start", () => {
    const format = formatCounts([grid.min, grid.max])
    expect(cellText(grid, { row: 0, col: 2 }, format)).toBe(
      "2024-01-02 10:02:00 to 2024-01-02 10:03:00, bucket 0 - 1, count 3"
    )
  })

  it("names the series and value in ordinal mode, with a dash for gaps", () => {
    const ordinal = build([
      { ...series([start, start + minute], [1, null]), ref: "A" },
    ])
    const format = formatCounts([ordinal.min, ordinal.max])
    expect(cellText(ordinal, { row: 0, col: 1 }, format)).toMatch(
      /^2024-01-02 10:01:00 to 2024-01-02 10:02:00, series .+, value -$/
    )
  })

  it("gives a single time bucket only its start", () => {
    const single = build([series([start], [2], { le: "1" })])
    const format = formatCounts([single.min, single.max])
    expect(cellText(single, { row: 0, col: 0 }, format)).toBe(
      "2024-01-02 10:00:00, bucket 0 - 1, count 2"
    )
  })
})

describe("columnAt", () => {
  const xs = [1000, 2000, 3000, 4000]

  it("finds nothing without columns", () => {
    expect(columnAt([], 1000)).toBeNull()
  })

  it("matches a single column only at its own time", () => {
    expect(columnAt([5000], 5000)).toBe(0)
    expect(columnAt([5000], 5001)).toBeNull()
    expect(columnAt([5000], 4999)).toBeNull()
  })

  it("picks the nearest column", () => {
    expect(columnAt(xs, 1000)).toBe(0)
    expect(columnAt(xs, 2400)).toBe(1)
    expect(columnAt(xs, 2600)).toBe(2)
    expect(columnAt(xs, 4000)).toBe(3)
  })

  it("accepts up to half a step beyond each end", () => {
    expect(columnAt(xs, 500)).toBe(0)
    expect(columnAt(xs, 499)).toBeNull()
    expect(columnAt(xs, 4500)).toBe(3)
    expect(columnAt(xs, 4501)).toBeNull()
  })
})

describe("calculated time buckets", () => {
  const times = Array.from({ length: 10 }, (_, i) => i * 1000)
  const frame = series(
    times,
    times.map((_, i) => i)
  )

  function perColumn(display: Record<string, unknown>) {
    const model = build([frame], {}, { calculate: true, ...display })
    return {
      xs: model.xs,
      totals: model.xs.map((_, col) =>
        model.cells.reduce((sum, row) => sum + (row[col] ?? 0), 0)
      ),
    }
  }

  it("keeps one column per sample time without xBuckets", () => {
    expect(perColumn({}).xs).toEqual(times)
  })

  it("splits the time range into a number of columns", () => {
    const { xs, totals } = perColumn({ xBuckets: { mode: "count", value: 2 } })
    expect(xs).toEqual([0, 4500])
    expect(totals).toEqual([5, 5])
  })

  it("makes columns of a size in seconds aligned to multiples of it", () => {
    const { xs, totals } = perColumn({ xBuckets: { mode: "size", value: 3 } })
    expect(xs).toEqual([0, 3000, 6000, 9000])
    expect(totals).toEqual([3, 3, 3, 1])
  })

  it("aligns a size to its multiples even when the data starts between them", () => {
    const shifted = series([1500, 2500, 3500], [1, 2, 3])
    const model = build(
      [shifted],
      {},
      { calculate: true, xBuckets: { mode: "size", value: 2 } }
    )
    expect(model.xs).toEqual([0, 2000])
  })

  it("sizes columns by the data step when the size has no value", () => {
    const sparse = series([0, 2000, 4000], [1, 2, 3])
    const model = build(
      [sparse],
      {},
      { calculate: true, xBuckets: { mode: "size" } }
    )
    expect(model.xs).toEqual([0, 2000, 4000])
  })

  it("keeps the sample columns for a count without a value", () => {
    expect(perColumn({ xBuckets: { mode: "count" } }).xs).toEqual(times)
  })

  it("leaves a single sample time in one column", () => {
    const model = build(
      [series([5000], [3])],
      {},
      { calculate: true, xBuckets: { mode: "count", value: 4 } }
    )
    expect(model.xs).toEqual([5000])
  })
})

describe("calculated value buckets", () => {
  const values = [0, 4, 8, 12, 16]
  const frame = series(
    values.map((_, i) => i),
    values
  )

  function rowCounts(display: Record<string, unknown>, data = frame) {
    const model = build([data], {}, { calculate: true, ...display })
    return {
      model,
      counts: model.cells.map((row) =>
        row.reduce<number>((sum, value) => sum + (value ?? 0), 0)
      ),
    }
  }

  it("uses a number of rows", () => {
    const { model, counts } = rowCounts({
      yBuckets: { mode: "count", value: 4 },
    })
    expect(model.rows.map((row) => row.from)).toEqual([0, 4, 8, 12])
    expect(model.rows[3].to).toBe(16)
    expect(counts).toEqual([1, 1, 1, 2])
  })

  it("uses rows of a size whose edges are multiples of it", () => {
    const { model, counts } = rowCounts({
      yBuckets: { mode: "size", value: 5 },
    })
    expect(model.rows.map((row) => row.from)).toEqual([0, 5, 10, 15])
    expect(model.rows[3].to).toBe(20)
    expect(counts).toEqual([2, 1, 1, 1])
  })

  it("picks a round size for about 16 rows when the size has no value", () => {
    const wide = series([1, 2], [0, 100])
    const { model } = rowCounts({ yBuckets: { mode: "size" } }, wide)
    expect(model.rows).toHaveLength(10)
    expect(model.rows[1].from).toBe(10)
    expect(model.rows[9].to).toBe(100)
  })

  it("keeps 16 rows for a count without a value", () => {
    expect(rowCounts({ yBuckets: { mode: "count" } }).model.rows).toHaveLength(
      calculatedRows
    )
  })

  it("steps by powers of the base on a log scale", () => {
    const wide = series([1, 2, 3], [1, 50, 5000])
    const { model, counts } = rowCounts(
      {
        yBuckets: { mode: "size", value: 1 },
        yBucketScale: "log",
        yBucketLogBase: 10,
      },
      wide
    )
    expect(model.rows.map((row) => row.from)).toEqual(
      [1, 10, 100, 1000].map((edge) => expect.closeTo(edge, 6))
    )
    expect(counts).toEqual([1, 1, 0, 1])
    expect(model.cells[1]).toEqual([0, 1, 0])
    expect(model.cells[3]).toEqual([0, 0, 1])
  })

  it("puts zero in the middle row of a symmetric log scale", () => {
    const signed = series([1, 2, 3], [-10, 0, 10])
    const { model } = rowCounts({ yBucketScale: "symlog" }, signed)
    expect(model.cells[8][1]).toBe(1)
    expect(model.cells[0][0]).toBe(1)
    expect(model.cells[15][2]).toBe(1)
  })

  it("lets yBucketScale win over yScale", () => {
    const wide = series([1, 2], [1, 10000])
    const { model } = rowCounts({ yScale: "log", yBucketScale: "linear" }, wide)
    expect(model.rows[1].from).toBeCloseTo(625.9375, 3)
  })

  it("falls back to the base 2 when none is written", () => {
    const wide = series([1, 2], [1, 256])
    const { model } = rowCounts(
      { yBucketScale: "log", yBuckets: { mode: "size", value: 2 } },
      wide
    )
    expect(model.rows.map((row) => row.from)).toEqual(
      [1, 4, 16, 64].map((edge) => expect.closeTo(edge, 6))
    )
  })
})

describe("pre-bucketed layouts", () => {
  const cumulative = [
    series([1], [10], { le: "1" }),
    series([1], [6], { le: "5" }),
    series([1], [3], { le: "10" }),
  ]

  it("reads each bound as the upper edge for le", () => {
    const model = build(cumulative, {}, { rowsLayout: "le" })
    expect(model.rows.map(({ from, to }) => [from, to])).toEqual([
      [0, 1],
      [1, 5],
      [5, 10],
    ])
  })

  it("reads each bound as the lower edge for ge, the last row open above", () => {
    const model = build(cumulative, {}, { rowsLayout: "ge" })
    expect(model.mode).toBe("buckets")
    expect(model.rows.map(({ from, to }) => [from, to])).toEqual([
      [1, 5],
      [5, 10],
      [10, undefined],
    ])
    expect(model.rows.map((row) => row.range)).toEqual([
      "1 - 5",
      "5 - 10",
      "10 and above",
    ])
    expect(model.cells).toEqual([[10], [6], [3]])
  })

  it("de-accumulates from the highest bound for ge in the heatmap format", () => {
    const model = build(
      cumulative,
      { queries: bucketQuery },
      { rowsLayout: "ge" }
    )
    expect(model.cells).toEqual([[4], [3], [3]])
  })

  it("draws one ordinary row per series for unknown", () => {
    const model = build(cumulative, {}, { rowsLayout: "unknown" })
    expect(model.mode).toBe("ordinal")
    expect(model.rows).toHaveLength(3)
  })

  it("reads series named by a number as bounds only when the layout is written", () => {
    const named = (name: string, value: number): PreparedFrame => ({
      ref: "A",
      labels: {},
      fields: [
        { name: "Time", type: "time", values: [1] },
        { name, type: "number", values: [value] },
      ],
    })
    const frames = [named("1", 4), named("5", 2)]
    expect(build(frames).mode).toBe("ordinal")
    const model = build(frames, {}, { rowsLayout: "le" })
    expect(model.mode).toBe("buckets")
    expect(model.rows.map((row) => row.label)).toEqual(["1", "5"])
  })
})

describe("y axis rows", () => {
  const frames = [
    series([1], [1], { le: "1" }),
    series([1], [2], { le: "5" }),
    series([1], [3], { le: "10" }),
    series([1], [4], { le: "+Inf" }),
  ]
  const labels = (
    display: Record<string, unknown>,
    extra: Partial<DataPanel> = {}
  ) => build(frames, extra, display).rows.map((row) => row.label)

  it("drops rows that end at or below yMin and start at or above yMax", () => {
    expect(labels({ yMin: 1, yMax: 10 })).toEqual(["5", "10"])
  })

  it("keeps rows that only partly lie in the window", () => {
    expect(labels({ yMin: 2, yMax: 6 })).toEqual(["5", "10"])
  })

  it("keeps only the open top row above the last bound", () => {
    expect(labels({ yMin: 20, yMax: 30 })).toEqual(["+Inf"])
  })

  it("has no rows when the window holds none", () => {
    expect(labels({ yMin: 5, yMax: 5 })).toEqual([])
  })

  it("draws the lowest row last when reversed, with its cells", () => {
    const model = build(frames, {}, { yReverse: true })
    expect(model.reversed).toBe(true)
    expect(model.rows.map((row) => row.label)).toEqual(["+Inf", "10", "5", "1"])
    expect(model.cells).toEqual([[4], [3], [2], [1]])
  })

  it("formats labels with yUnit and yDecimals", () => {
    const model = build(
      [series([1], [1], { le: "0.5" }), series([1], [1], { le: "+Inf" })],
      {},
      { yUnit: "percentunit", yDecimals: 1 }
    )
    expect(model.rows[0].label).toBe("50.0%")
  })

  it("formats calculated rows with yDecimals", () => {
    const model = build(
      [series([1, 2], [0, 1])],
      {},
      { calculate: true, yDecimals: 3, yBuckets: { mode: "count", value: 4 } }
    )
    expect(model.rows.map((row) => row.label)).toEqual([
      "0.000",
      "0.250",
      "0.500",
      "0.750",
    ])
  })
})
