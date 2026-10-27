import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  calculatedRows,
  cellOpacity,
  heatmapModel,
  legendScale,
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

describe("colour scale", () => {
  it("runs from 10% to 100% opacity", () => {
    expect(cellOpacity(0, 10)).toBeCloseTo(0.1)
    expect(cellOpacity(10, 10)).toBeCloseTo(1)
    expect(cellOpacity(5, 10)).toBeCloseTo(0.55)
  })

  it("steps the legend from the lowest to the highest count", () => {
    const steps = legendScale(1, 5)
    expect(steps.map((step) => step.value)).toEqual([1, 2, 3, 4, 5])
    expect(steps[4].opacity).toBeCloseTo(1)
  })
})
