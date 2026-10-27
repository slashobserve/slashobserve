import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  axisTicks,
  missingFieldsReason,
  nearestPoint,
  pointCount,
  pointsAtX,
  pointSizeOf,
  showOf,
  xyModel,
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
  display: Record<string, unknown> = {}
) {
  return xyModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("xychart", display)
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
    expect(model.x).toEqual({ min: -5, max: 105 })
    expect(model.y).toEqual({ min: -0.5, max: 10.5 })
    const widened = build([frame({ x: [0, 100], y: [0, 10] })], {
      min: -20,
      max: 500,
    })
    expect(widened.y).toEqual({ min: -20, max: 500 })
    expect(widened.x).toEqual({ min: -20, max: 500 })
  })

  it("gives a single point a visible range", () => {
    const model = build([frame({ x: [5], y: [7] })])
    expect(model.x.max).toBeGreaterThan(model.x.min)
    expect(model.y.max).toBeGreaterThan(model.y.min)
  })
})

describe("display options", () => {
  it("reads show and clamps the point size", () => {
    expect(showOf({ show: "lines" })).toBe("lines")
    expect(showOf({})).toBe("points")
    expect(pointSizeOf({ pointSize: 99 })).toBe(40)
    expect(pointSizeOf({ pointSize: 0 })).toBe(1)
    expect(pointSizeOf({})).toBe(5)
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
    const px = pixel(100, model.x, scale.width)
    const py = scale.height - pixel(100, model.y, scale.height)
    const hit = nearestPoint(model, px + 2, py - 1, scale)
    expect(hit).toMatchObject({ series: 0, index: 2 })
  })

  it("returns null when no point is within the radius", () => {
    expect(nearestPoint(model, 5, 5, { ...scale, radius: 3 })).toBeNull()
  })

  it("picks the closest series where points share an x", () => {
    const px = pixel(50, model.x, scale.width)
    const py = scale.height - pixel(50, model.y, scale.height)
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
    const px = pixel(target, big.x, scale.width)
    const py = scale.height - pixel(ys[target], big.y, scale.height)
    const hit = nearestPoint(big, px, py, { ...scale, radius: 4 })
    expect(hit).not.toBeNull()
    const [x, y] = big.series[0].points[hit!.index]
    const hx = pixel(x, big.x, scale.width)
    const hy = scale.height - pixel(y, big.y, scale.height)
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
        const dx = pixel(x, sample.x, scale.width) - px
        const dy = scale.height - pixel(y, sample.y, scale.height) - py
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
