import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  axisFraction,
  axisTicks,
  missingFieldsReason,
  nearestPoint,
  pointCount,
  pointsAtX,
  pointSummary,
  tooltipHits,
  xyData,
  xyModel,
  xyOptions,
} from "./xy-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "XY",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "xychart",
    datasource: "ch",
    queries: [{ query: "select 1" }],
    ...extra,
  } as DataPanel
}

function frame(
  columns: Record<string, (number | null)[]>,
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return {
    ref: "A",
    labels: {},
    fields: Object.entries(columns).map(([name, values]) => ({
      name,
      type: "number" as const,
      values,
    })),
    ...extra,
  }
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {},
  hidden: ReadonlySet<string> = new Set()
) {
  return xyModel(
    xyData(
      frames,
      createFieldResolver(panel(extra)),
      displayWithDefaults("xychart", display)
    ),
    hidden
  )
}

const scale = { width: 400, height: 200, radius: 20 }

describe("xyModel", () => {
  it("uses the first number field as x when xField is absent", () => {
    const model = build([frame({ size: [1, 2, 3], latency: [10, 20, 30] })])
    expect(model.reason).toBeNull()
    expect(model.series).toHaveLength(1)
    expect(model.series[0].points.map(([x, y]) => [x, y])).toEqual([
      [1, 10],
      [2, 20],
      [3, 30],
    ])
  })

  it("uses the named field as x and plots every other number field", () => {
    const model = build(
      [frame({ a: [1, 2], b: [5, 6], c: [7, 8] })],
      {},
      { xField: "b" }
    )
    expect(model.series.map((series) => series.name)).toEqual(["a", "c"])
    expect(model.series[0].points.map(([x]) => x)).toEqual([5, 6])
  })

  it("draws nothing for a frame without the named x field", () => {
    const model = build([frame({ a: [1], b: [2] })], {}, { xField: "z" })
    expect(model.series).toHaveLength(0)
    expect(model.reason).toBe(missingFieldsReason)
  })

  it("keeps one series per y field across two frames", () => {
    const model = build([
      frame({ size: [1, 2], latency: [10, 20] }, { ref: "A" }),
      frame({ size: [3, 4], errors: [1, 2] }, { ref: "B" }),
    ])
    expect(model.series.map((series) => series.name)).toEqual([
      "latency",
      "errors",
    ])
    expect(new Set(model.series.map((series) => series.color)).size).toBe(2)
  })

  it("explains that a frame with one number field cannot be drawn", () => {
    const model = build([frame({ only: [1, 2, 3] })])
    expect(model.series).toHaveLength(0)
    expect(model.reason).toBe(missingFieldsReason)
  })

  it("drops pairs that are not finite numbers", () => {
    const model = build([
      frame({ x: [1, null, 3, Infinity, 5], y: [1, 2, NaN, 4, 5] }),
    ])
    expect(model.series[0].points.map(([x, y]) => [x, y])).toEqual([
      [1, 1],
      [5, 5],
    ])
  })

  it("skips hidden y fields", () => {
    const model = build([frame({ x: [1, 2], keep: [1, 2], drop: [3, 4] })], {
      overrides: [{ match: { field: "drop" }, hidden: true }],
    } as Partial<DataPanel>)
    expect(model.series.map((series) => series.name)).toEqual(["keep"])
  })

  it("colors each point by the threshold of its y value", () => {
    const model = build([frame({ x: [1, 2, 3], y: [10, 60, 95] })], {
      thresholds: [
        { color: "ok" },
        { value: 50, color: "warning" },
        { value: 90, color: "critical" },
      ],
    } as Partial<DataPanel>)
    const colors = model.series[0].points.map(([, , color]) => color)
    expect(colors).toEqual([
      "var(--threshold-ok)",
      "var(--threshold-warning)",
      "var(--threshold-critical)",
    ])
  })

  it("colors points by the series color when nothing else applies", () => {
    const model = build([frame({ x: [1, 2], y: [1, 2] })])
    const [series] = model.series
    expect(series.points.every(([, , color]) => color === series.color)).toBe(
      true
    )
  })

  it("sorts points by x for lines and keeps the order for points", () => {
    const columns = { x: [3, 1, 2], y: [30, 10, 20] }
    const lines = build([frame(columns)], {}, { show: "lines" })
    expect(lines.series[0].points.map(([x]) => x)).toEqual([1, 2, 3])
    const both = build([frame(columns)], {}, { show: "points+lines" })
    expect(both.series[0].points.map(([x]) => x)).toEqual([1, 2, 3])
    const points = build([frame(columns)])
    expect(points.series[0].points.map(([x]) => x)).toEqual([3, 1, 2])
  })

  it("pads ranges by 5 percent and widens them to field min and max", () => {
    const model = build([frame({ x: [0, 100], y: [0, 10] })])
    expect(model.x.range).toEqual({ min: -5, max: 105 })
    expect(model.y.range).toEqual({ min: -0.5, max: 10.5 })
    const widened = build([frame({ x: [0, 100], y: [0, 10] })], {
      min: -20,
      max: 500,
    })
    expect(widened.y.range).toEqual({ min: -20, max: 500 })
    expect(widened.x.range).toEqual({ min: -20, max: 500 })
  })

  it("gives a single point a visible range", () => {
    const model = build([frame({ x: [5], y: [7] })])
    expect(model.x.range.max).toBeGreaterThan(model.x.range.min)
    expect(model.y.range.max).toBeGreaterThan(model.y.range.min)
  })
})

describe("xyOptions", () => {
  it("reads show, sizes and the shared legend and tooltip keys", () => {
    expect(xyOptions({ show: "lines" }).show).toBe("lines")
    expect(xyOptions({}).show).toBe("points")
    expect(xyOptions({}).pointSize).toBe(5)
    expect(xyOptions({ pointSizeMin: 20, pointSizeMax: 6 })).toMatchObject({
      pointSizeMin: 6,
      pointSizeMax: 20,
    })
    expect(
      xyOptions({
        legendMode: "table",
        legendValues: ["max"],
        legendSortBy: "max",
        legendSortDesc: true,
        legendWidth: 200,
        tooltipSort: "desc",
        tooltipHideZeros: true,
        tooltipMaxWidth: 300,
        tooltipMaxHeight: 120,
      })
    ).toMatchObject({
      legendMode: "table",
      legendValues: ["max"],
      legendSortBy: "max",
      legendSortDesc: true,
      legendWidth: 200,
      tooltipSort: "desc",
      tooltipHideZeros: true,
      tooltipMaxWidth: 300,
      tooltipMaxHeight: 120,
    })
  })
})

describe("nearestPoint", () => {
  const model = build([
    frame({ x: [0, 50, 100], a: [0, 50, 100], b: [100, 50, 0] }),
  ])
  const pixel = (
    value: number,
    range: { min: number; max: number },
    size: number
  ) => ((value - range.min) / (range.max - range.min)) * size

  it("finds the point under the pointer", () => {
    const px = pixel(100, model.x.range, scale.width)
    const py = scale.height - pixel(100, model.y.range, scale.height)
    const hit = nearestPoint(model, px + 2, py - 1, scale)
    expect(hit).toMatchObject({ series: 0, index: 2 })
  })

  it("returns null when no point is within the radius", () => {
    expect(nearestPoint(model, 5, 5, { ...scale, radius: 3 })).toBeNull()
  })

  it("picks the closest series where points share an x", () => {
    const px = pixel(50, model.x.range, scale.width)
    const py = scale.height - pixel(50, model.y.range, scale.height)
    const hit = nearestPoint(model, px, py, scale)
    expect(hit?.index).toBe(1)
    expect(hit?.distance).toBeLessThan(1)
  })

  it("finds a nearest point among 100,000", () => {
    const count = 100_000
    const xs = Array.from({ length: count }, (_, i) => i)
    const ys = xs.map((i) => (i * 7919) % 1000)
    const big = build([frame({ x: xs, y: ys })])
    expect(pointCount(big)).toBe(count)
    const target = 54_321
    const px = pixel(target, big.x.range, scale.width)
    const py = scale.height - pixel(ys[target], big.y.range, scale.height)
    const hit = nearestPoint(big, px, py, { ...scale, radius: 4 })
    expect(hit).not.toBeNull()
    const [x, y] = big.series[0].points[hit!.index]
    const hx = pixel(x, big.x.range, scale.width)
    const hy = scale.height - pixel(y, big.y.range, scale.height)
    expect(Math.hypot(hx - px, hy - py)).toBeCloseTo(hit!.distance, 6)
  })

  it("agrees with a brute-force search", () => {
    const count = 2_000
    const xs = Array.from({ length: count }, (_, i) => (i * 37) % 997)
    const ys = Array.from({ length: count }, (_, i) => (i * 91) % 503)
    const sample = build([frame({ x: xs, y: ys })])
    for (const [px, py] of [
      [10, 10],
      [200, 100],
      [399, 199],
      [123, 45],
    ]) {
      let best = Infinity
      for (const [x, y] of sample.series[0].points) {
        const dx = pixel(x, sample.x.range, scale.width) - px
        const dy = scale.height - pixel(y, sample.y.range, scale.height) - py
        best = Math.min(best, Math.hypot(dx, dy))
      }
      const hit = nearestPoint(sample, px, py, {
        ...scale,
        radius: Number.POSITIVE_INFINITY,
      })
      expect(hit?.distance).toBeCloseTo(best, 6)
    }
  })
})

describe("pointsAtX", () => {
  it("gives each series its point nearest to the x", () => {
    const model = build([
      frame({ x: [1, 2, 3], a: [10, 20, 30] }),
      frame({ x: [1, 4], b: [5, 6] }, { ref: "B" }),
    ])
    const hits = pointsAtX(model, 3.4)
    expect(hits.map(({ series, index }) => [series, index])).toEqual([
      [0, 2],
      [1, 1],
    ])
  })
})

describe("axisTicks", () => {
  it("rounds ticks to nice values and formats them with the unit", () => {
    const ticks = axisTicks({ min: 0, max: 1000 }, { unit: "ms" } as never, 5)
    expect(ticks.map(({ value }) => value)).toEqual([
      0, 200, 400, 600, 800, 1000,
    ])
    expect(ticks[1].text).toBe("200 ms")
  })

  it("widens decimals until neighbouring ticks differ", () => {
    const ticks = axisTicks({ min: 0, max: 0.5 }, undefined, 5)
    const texts = ticks.map(({ text }) => text)
    expect(new Set(texts).size).toBe(texts.length)
  })

  it("returns no ticks for an empty range", () => {
    expect(axisTicks({ min: 1, max: 1 }, undefined, 5)).toEqual([])
  })
})

describe("series mapping", () => {
  const thresholds = [
    { color: "ok" },
    { value: 50, color: "warning" },
    { value: 90, color: "critical" },
  ]

  it("builds one series per mapping entry", () => {
    const model = build(
      [frame({ x: [1, 2], a: [3, 4], b: [5, 6] })],
      {},
      {
        series: [
          { x: "x", y: "a" },
          { x: "x", y: "b", name: "Beta" },
        ],
      }
    )
    expect(model.series.map((series) => series.name)).toEqual(["a", "Beta"])
    expect(model.series[1].points.map(([, y]) => y)).toEqual([5, 6])
  })

  it("prefixes the mapping name when it covers several y fields", () => {
    const model = build(
      [frame({ x: [1], a: [3], b: [5] })],
      {},
      { series: [{ name: "Run" }] }
    )
    expect(model.series.map((series) => series.name)).toEqual([
      "Run a",
      "Run b",
    ])
  })

  it("keeps the size and color fields out of the y fields", () => {
    const model = build(
      [frame({ x: [1], y: [2], n: [3], c: [4] })],
      {},
      { series: [{ size: "n", color: "c" }] }
    )
    expect(model.series.map((series) => series.yName)).toEqual(["y"])
  })

  it("matches y fields by exact name or by slashed regex", () => {
    const frames = [frame({ x: [1], latency: [1], errors: [2], lag: [3] })]
    const exact = build(frames, {}, { series: [{ y: "errors" }] })
    expect(exact.series.map((series) => series.yName)).toEqual(["errors"])
    const regex = build(frames, {}, { series: [{ y: "/^la/" }] })
    expect(regex.series.map((series) => series.yName)).toEqual([
      "latency",
      "lag",
    ])
  })

  it("filters frames by query ref and by frame name", () => {
    const frames = [
      frame({ x: [1], a: [1] }, { ref: "A", name: "alpha" }),
      frame({ x: [1], b: [2] }, { ref: "B", name: "beta" }),
    ]
    const byQuery = build(frames, {}, { series: [{ query: "B" }] })
    expect(byQuery.series.map((series) => series.yName)).toEqual(["b"])
    const byName = build(frames, {}, { series: [{ frame: "alpha" }] })
    expect(byName.series.map((series) => series.yName)).toEqual(["a"])
    const byRegex = build(frames, {}, { series: [{ frame: "/^be/" }] })
    expect(byRegex.series.map((series) => series.yName)).toEqual(["b"])
  })

  it("uses the mapping x over the display x field", () => {
    const model = build(
      [frame({ a: [1], b: [2], c: [3] })],
      {},
      { xField: "a", series: [{ x: "b", y: "c" }] }
    )
    expect(model.series[0].points[0].slice(0, 2)).toEqual([2, 3])
  })

  it("explains an empty result when no mapping finds fields", () => {
    const model = build(
      [frame({ x: [1], y: [2] })],
      {},
      { series: [{ x: "missing" }] }
    )
    expect(model.series).toHaveLength(0)
    expect(model.reason).toBe(missingFieldsReason)
  })

  it("scales point diameter between the minimum and maximum by the size field", () => {
    const model = build(
      [frame({ x: [1, 2, 3], y: [1, 2, 3], n: [0, 50, 100] })],
      {},
      {
        pointSizeMin: 4,
        pointSizeMax: 24,
        series: [{ x: "x", y: "y", size: "n" }],
      }
    )
    expect(model.series[0].points.map((point) => point[3])).toEqual([4, 14, 24])
    expect(model.series[0].points.map((point) => point[4])).toEqual([
      0, 50, 100,
    ])
  })

  it("scales against the size field min and max when it declares them", () => {
    const model = build(
      [frame({ x: [1, 2], y: [1, 2], n: [0, 100] })],
      {
        overrides: [{ match: { field: "n" }, min: 0, max: 200 }],
      } as Partial<DataPanel>,
      { pointSizeMin: 4, pointSizeMax: 24, series: [{ size: "n" }] }
    )
    expect(model.series[0].points.map((point) => point[3])).toEqual([4, 14])
  })

  it("uses the fixed point size without a size field", () => {
    const model = build([frame({ x: [1], y: [2] })], {}, { pointSize: 9 })
    expect(model.series[0].points[0][3]).toBe(9)
  })

  it("colors points by the color field instead of the y value", () => {
    const model = build(
      [frame({ x: [1, 2, 3], y: [1, 2, 3], c: [10, 60, 95] })],
      { thresholds } as Partial<DataPanel>,
      { series: [{ color: "c" }] }
    )
    expect(model.series[0].points.map((point) => point[2])).toEqual([
      "var(--threshold-ok)",
      "var(--threshold-warning)",
      "var(--threshold-critical)",
    ])
  })

  it("drops series the legend hides", () => {
    const frames = [frame({ x: [1, 2], a: [1, 2], b: [3, 4] })]
    const all = build(frames)
    const hidden = build(frames, {}, {}, new Set([all.series[0].key]))
    expect(hidden.series.map((series) => series.yName)).toEqual(["b"])
    expect(hidden.y.name).toBe("b")
  })
})

describe("series drawing", () => {
  const frames = [frame({ x: [1, 2], y: [1, 2] })]

  it("draws lines 1.5 wide unless the field sets a width", () => {
    expect(build(frames).series[0].lineWidth).toBe(1.5)
    expect(
      build(frames, { lineWidth: 3 } as Partial<DataPanel>).series[0].lineWidth
    ).toBe(3)
  })

  it("reads the dash, point shape, stroke and fill", () => {
    const [plain] = build(frames).series
    expect(plain).toMatchObject({
      dash: [],
      square: false,
      strokeWidth: 0,
      fillAlpha: 1,
    })
    const [styled] = build(frames, {
      lineStyle: "dashed",
      pointShape: "square",
      pointStrokeWidth: 2,
      fill: 40,
    } as Partial<DataPanel>).series
    expect(styled.dash.length).toBeGreaterThan(0)
    expect(styled).toMatchObject({
      square: true,
      strokeWidth: 2,
      fillAlpha: 0.4,
    })
  })

  it("takes both axes from the first visible series", () => {
    const model = build(frames, {
      axis: "right",
      axisLabel: "Latency",
      axisBorder: true,
    } as Partial<DataPanel>)
    expect(model.y.settings).toMatchObject({
      side: "end",
      label: "Latency",
      border: true,
    })
  })

  it("drops points at or below zero on a log axis", () => {
    const model = build([frame({ x: [0, 1, 10, 100], y: [1, 2, 3, 4] })], {
      scale: "log",
    } as Partial<DataPanel>)
    expect(model.series[0].points.map(([x]) => x)).toEqual([1, 10, 100])
    expect(model.x.scale.kind).toBe("log")
    expect(model.x.range.min).toBeGreaterThan(0)
  })
})

describe("log axes", () => {
  it("places points by their position on the scale", () => {
    const model = build([frame({ x: [1, 10, 100], y: [1, 2, 3] })], {
      scale: "log",
    } as Partial<DataPanel>)
    const pixelX = axisFraction(model.x, 10) * scale.width
    const pixelY = (1 - axisFraction(model.y, 2)) * scale.height
    expect(nearestPoint(model, pixelX, pixelY, scale)).toMatchObject({
      series: 0,
      index: 1,
    })
  })

  it("ticks at powers of the base", () => {
    const ticks = axisTicks({ min: 1, max: 1000 }, undefined, 5, {
      kind: "log",
      logBase: 10,
      linearThreshold: 1,
    })
    expect(ticks.map(({ value }) => value)).toEqual([1, 10, 100, 1000])
  })
})

describe("tooltip", () => {
  const frames = [frame({ x: [1], a: [5], b: [0], c: [9] })]
  const hits = [
    { series: 0, index: 0, distance: 0 },
    { series: 1, index: 0, distance: 0 },
    { series: 2, index: 0, distance: 0 },
  ]

  it("keeps series order by default", () => {
    const model = build(frames)
    const kept = tooltipHits(model, hits, xyOptions({}))
    expect(kept.map(({ series }) => series)).toEqual([0, 1, 2])
  })

  it("sorts by value and hides zeros", () => {
    const model = build(frames)
    const kept = tooltipHits(
      model,
      hits,
      xyOptions({ tooltipSort: "desc", tooltipHideZeros: true })
    )
    expect(kept.map(({ series }) => series)).toEqual([2, 0])
  })

  it("leaves out series hidden from the tooltip", () => {
    const model = build(frames, {
      overrides: [{ match: { field: "a" }, hideInTooltip: true }],
    } as Partial<DataPanel>)
    const kept = tooltipHits(model, hits, xyOptions({}))
    expect(kept.map(({ series }) => series)).toEqual([1, 2])
  })

  it("lists the size and color fields beside x and y", () => {
    const model = build(
      [frame({ x: [1], y: [2], n: [30], c: [40] })],
      { unit: "ms" } as Partial<DataPanel>,
      { series: [{ size: "n", color: "c" }] }
    )
    expect(pointSummary(model, { series: 0, index: 0 })).toEqual({
      x: "1 ms",
      y: "2 ms",
      extras: [
        { name: "n", text: "30 ms" },
        { name: "c", text: "40 ms" },
      ],
    })
  })
})
