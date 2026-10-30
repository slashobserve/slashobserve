import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  countTicks,
  histogramEntries,
  histogramModel,
  histogramOptions,
  labelStride,
  tooltipSeries,
  valueTicks,
} from "./histogram-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Histogram",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "histogram",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function series(
  name: string,
  values: (number | null)[],
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return {
    ref: "A",
    name,
    labels: {},
    fields: [
      { name: "Time", type: "time", values: values.map((_, i) => i) },
      { name: "Value", type: "number", values },
    ],
    ...extra,
  }
}

function build(
  frames: PreparedFrame[],
  display: Record<string, unknown> = {},
  extra: Partial<DataPanel> = {}
) {
  return buildHidden(frames, display, extra)
}

function buildHidden(
  frames: PreparedFrame[],
  display: Record<string, unknown> = {},
  extra: Partial<DataPanel> = {},
  hidden: ReadonlySet<string> = new Set()
) {
  const merged = displayWithDefaults("histogram", display)
  const entries = histogramEntries(
    frames,
    createFieldResolver(panel(extra)),
    histogramOptions(merged).combine
  )
  return histogramModel(entries, merged, hidden)
}

const zeroToHundred = Array.from({ length: 101 }, (_, i) => i)

describe("histogramModel", () => {
  it("uses 20 equal buckets between the minimum and the maximum", () => {
    const model = build([series("a", zeroToHundred)])
    expect(model.edges).toHaveLength(21)
    expect(model.edges[0]).toBe(0)
    expect(model.edges[1]).toBe(5)
    expect(model.edges[20]).toBe(100)
    expect(model.series[0].counts).toHaveLength(20)
  })

  it("counts every value once, with the maximum in the last bucket", () => {
    const model = build([series("a", zeroToHundred)])
    const { counts } = model.series[0]
    expect(counts.reduce((sum, count) => sum + count, 0)).toBe(101)
    expect(model.total).toBe(101)
    expect(counts[0]).toBe(5)
    expect(counts[19]).toBe(6)
  })

  it("aligns bucket edges to multiples of bucketSize", () => {
    const model = build([series("a", [7, 12, 18, 33])], { bucketSize: 10 })
    expect(model.edges).toEqual([0, 10, 20, 30, 40])
    expect(model.series[0].counts).toEqual([1, 2, 0, 1])
  })

  it("starts aligned buckets below a negative minimum", () => {
    const model = build([series("a", [-12, -3, 4])], { bucketSize: 5 })
    expect(model.edges[0]).toBe(-15)
    expect(model.edges.at(-1)).toBe(5)
    expect(model.series[0].counts).toEqual([1, 0, 1, 1])
  })

  it("keeps a value on the last aligned edge in the last bucket", () => {
    const model = build([series("a", [0, 5, 10])], { bucketSize: 5 })
    expect(model.edges).toEqual([0, 5, 10])
    expect(model.series[0].counts).toEqual([1, 2])
  })

  it.each([
    ["a size so small the bucket count overflows", [0, 1e10], 1e-300],
    ["a size so small the aligned start overflows", [1e10, 2e10], 1e-300],
    ["a size that needs more than the maximum bucket count", [0, 1e6], 1],
  ])("stays within the bucket cap for %s", (_name, values, bucketSize) => {
    const model = build([series("a", values)], { bucketSize })
    expect(model.edges.length).toBeLessThanOrEqual(1001)
    expect(model.edges.every(Number.isFinite)).toBe(true)
    expect(model.total).toBe(2)
  })

  it("prefers bucketSize over bucketCount", () => {
    const model = build([series("a", [0, 100])], {
      bucketSize: 50,
      bucketCount: 7,
    })
    expect(model.edges).toEqual([0, 50, 100])
  })

  it("uses bucketCount equal buckets", () => {
    const model = build([series("a", zeroToHundred)], { bucketCount: 10 })
    expect(model.edges).toHaveLength(11)
    expect(model.series[0].counts).toHaveLength(10)
    expect(model.edges[1]).toBe(10)
  })

  it("makes one bucket when every value is equal", () => {
    const model = build([series("a", [4, 4, 4])])
    expect(model.edges).toEqual([4, 4])
    expect(model.series[0].counts).toEqual([3])
    expect(build([series("a", [4, 4])], { bucketCount: 10 }).edges).toEqual([
      4, 4,
    ])
  })

  it("makes one aligned bucket when every value is equal", () => {
    const model = build([series("a", [4, 4])], { bucketSize: 5 })
    expect(model.edges).toEqual([0, 5])
    expect(model.series[0].counts).toEqual([2])
  })

  it("never makes more than 1000 buckets", () => {
    const model = build([series("a", [0, 1_000_000])], { bucketSize: 1 })
    expect(model.series[0].counts.length).toBeLessThanOrEqual(1000)
    expect(model.edges[0]).toBe(0)
    expect(model.edges.at(-1)).toBeGreaterThanOrEqual(1_000_000)
    expect(model.series[0].counts.reduce((sum, c) => sum + c, 0)).toBe(2)
  })

  it("skips hidden series", () => {
    const model = build(
      [series("a", [1, 2]), series("b", [100, 200], { ref: "B" })],
      {},
      { overrides: [{ match: { series: "b" }, hidden: true }] }
    )
    expect(model.series.map((s) => s.name)).toEqual(["a"])
    expect(model.edges[0]).toBe(1)
    expect(model.edges.at(-1)).toBe(2)
    expect(model.total).toBe(2)
  })

  it("ignores null and non-finite values", () => {
    const model = build([series("a", [null, 1, Infinity, 3, NaN])])
    expect(model.total).toBe(2)
    expect(model.edges[0]).toBe(1)
    expect(model.edges.at(-1)).toBe(3)
  })

  it("shares buckets between series and keeps their order", () => {
    const model = build(
      [series("a", [0, 10]), series("b", [5, 10], { ref: "B" })],
      { bucketCount: 2 }
    )
    expect(model.series.map((s) => s.name)).toEqual(["a", "b"])
    expect(model.series[0].counts).toEqual([1, 1])
    expect(model.series[1].counts).toEqual([0, 2])
    expect(model.total).toBe(4)
    expect(model.bars[1][1].end).toBe(3)
  })

  it("colors by series slot unless the field sets a color", () => {
    const model = build(
      [series("a", [1, 2]), series("b", [1, 2], { ref: "B" })],
      {},
      { overrides: [{ match: { series: "b" }, color: "critical" }] }
    )
    expect(model.series[0].color).toBe("var(--series-1)")
    expect(model.series[1].color).toBe("var(--threshold-critical)")
  })

  it("takes the unit of the first series and shares decimals across edges", () => {
    const model = build(
      [series("a", [0, 0.25])],
      { bucketCount: 4 },
      {
        unit: "percentunit",
      }
    )
    expect(model.unit).toBe("percentunit")
    expect(model.decimals).toBe(2)
  })

  it("returns no buckets without values", () => {
    const model = build([series("a", [null, null])])
    expect(model.edges).toEqual([])
    expect(model.series[0].counts).toEqual([])
    expect(model.total).toBe(0)
    expect(model.bars).toEqual([])
  })
})

describe("countTicks", () => {
  it("steps by whole numbers that cover the maximum", () => {
    expect(countTicks(7, 10)).toEqual([0, 1, 2, 3, 4, 5, 6, 7])
    expect(countTicks(7, 4)).toEqual([0, 2, 4, 6, 8])
    expect(countTicks(120, 5)).toEqual([0, 50, 100, 150])
  })

  it("always offers 0 and 1 at least", () => {
    expect(countTicks(0, 5)).toEqual([0, 1])
    expect(countTicks(1, 0)).toEqual([0, 1])
  })

  it("keeps every tick an integer", () => {
    for (const max of [1, 3, 9, 13, 97, 1234]) {
      for (const tick of countTicks(max, 6)) {
        expect(Number.isInteger(tick)).toBe(true)
      }
    }
  })
})

describe("labelStride", () => {
  it("labels every edge when they fit", () => {
    expect(labelStride(5, 400, 40)).toBe(1)
  })

  it("skips edges until labels no longer overlap", () => {
    expect(labelStride(21, 400, 60)).toBe(3)
  })

  it("is 1 without a plot", () => {
    expect(labelStride(1, 400, 60)).toBe(1)
    expect(labelStride(10, 0, 60)).toBe(1)
  })
})

describe("bucketOffset", () => {
  it("shifts aligned edges by the offset", () => {
    const model = build([series("a", [7, 12, 18, 33])], {
      bucketSize: 10,
      bucketOffset: 5,
    })
    expect(model.edges).toEqual([5, 15, 25, 35])
    expect(model.series[0].counts).toEqual([2, 1, 1])
  })

  it("aligns count based buckets to the offset too", () => {
    const model = build([series("a", [0, 10])], {
      bucketCount: 2,
      bucketOffset: 2,
    })
    expect(model.edges).toEqual([-3, 2, 7, 12])
  })
})

describe("combine", () => {
  it("merges every series into one", () => {
    const model = build(
      [series("a", [1, 2]), series("b", [3, 4], { ref: "B" })],
      { combine: true, bucketCount: 3 }
    )
    expect(model.series.map((s) => s.name)).toEqual(["All series"])
    expect(model.total).toBe(4)
    expect(model.series[0].counts.reduce((sum, c) => sum + c, 0)).toBe(4)
  })
})

describe("stacking", () => {
  const frames = [series("a", [0, 10]), series("b", [10, 10], { ref: "B" })]
  const display = { bucketCount: 2 }

  it("stacks series on top of each other by default", () => {
    const model = build(frames, display)
    expect(model.stack).toBe("normal")
    expect(model.bars[1][1]).toEqual({ start: 1, end: 3 })
    expect(model.max).toBeGreaterThanOrEqual(3)
  })

  it("overlays series from zero when not stacked", () => {
    const model = build(frames, { ...display, stack: "none" })
    expect(model.bars[1][1]).toEqual({ start: 0, end: 2 })
  })

  it("scales each bucket to 100 percent", () => {
    const model = build(frames, { ...display, stack: "percent" })
    expect(model.bars[0][1].end).toBeCloseTo(100 / 3)
    expect(model.bars[1][1].end).toBeCloseTo(100)
    expect([model.min, model.max]).toEqual([0, 100])
  })
})

describe("hidden series", () => {
  it("keeps the buckets and drops the series from the stack", () => {
    const frames = [series("a", [0, 10]), series("b", [10, 10], { ref: "B" })]
    const all = build(frames, { bucketCount: 2 })
    const model = buildHidden(
      frames,
      { bucketCount: 2 },
      {},
      new Set([all.series[0].key])
    )
    expect(model.edges).toEqual(all.edges)
    expect(model.series.map((s) => s.name)).toEqual(["b"])
    expect(model.bars[0][1]).toEqual({ start: 0, end: 2 })
  })
})

describe("value axis", () => {
  const frames = [series("a", [1, 1, 1, 9, 9])]

  it("counts in whole steps on the linear axis", () => {
    const model = build(frames, { bucketCount: 2 })
    const axis = valueTicks(model, 10)
    expect(axis.min).toBe(0)
    expect(axis.ticks).toEqual([0, 1, 2, 3])
  })

  it("widens to a soft maximum", () => {
    const model = build(frames, { bucketCount: 2 }, {
      axisSoftMax: 10,
    } as Partial<DataPanel>)
    expect(valueTicks(model, 10).max).toBe(10)
  })

  it("uses powers of ten on a log axis", () => {
    const model = build(
      [series("a", [...Array(500).fill(1), ...Array(5).fill(9)])],
      { bucketCount: 2 },
      { scale: "log" } as Partial<DataPanel>
    )
    expect(valueTicks(model, 10).ticks).toEqual([1, 10, 100, 1000])
  })

  it("moves the axis to the right side", () => {
    const model = build(frames, {}, { axis: "right" } as Partial<DataPanel>)
    expect(model.axis.settings.side).toBe("end")
  })
})

describe("fill and border", () => {
  it("is opaque with no border unless written", () => {
    const plain = build([series("a", [1, 2])]).series[0]
    expect([plain.fillOpacity, plain.borderWidth]).toEqual([1, 0])
    const written = build([series("a", [1, 2])], {}, {
      fill: 50,
      lineWidth: 2,
    } as Partial<DataPanel>).series[0]
    expect([written.fillOpacity, written.borderWidth]).toEqual([0.5, 2])
  })
})

describe("tooltipSeries", () => {
  const model = build(
    [
      series("a", [1, 1, 5]),
      series("b", [1, 5, 5], { ref: "B" }),
      series("c", [5, 5, 5], { ref: "C" }),
    ],
    { bucketCount: 2 }
  )
  const names = (rows: { index: number }[]) =>
    rows.map(({ index }) => model.series[index].name)
  const all = {
    tooltip: "all",
    tooltipSort: "none",
    tooltipHideZeros: false,
  } as const

  it("lists every series in all mode with the bucket total", () => {
    const { rows, total } = tooltipSeries(
      model,
      { index: 0, series: null },
      all
    )
    expect(names(rows)).toEqual(["a", "b", "c"])
    expect(total).toBe(rows.reduce((sum, row) => sum + row.value, 0))
  })

  it("lists only the hovered series in single mode", () => {
    const { rows } = tooltipSeries(
      model,
      { index: 0, series: model.series[1].key },
      { ...all, tooltip: "single" }
    )
    expect(names(rows)).toEqual(["b"])
  })

  it("sorts by count and can drop empty rows", () => {
    const sorted = tooltipSeries(
      model,
      { index: 0, series: null },
      { ...all, tooltipSort: "desc" }
    )
    expect(sorted.rows.map(({ value }) => value)).toEqual(
      [...sorted.rows.map(({ value }) => value)].sort((a, b) => b - a)
    )
    const empty = tooltipSeries(
      build([series("a", [0, 10]), series("b", [10, 10], { ref: "B" })], {
        bucketCount: 2,
      }),
      { index: 0, series: null },
      { ...all, tooltipHideZeros: true }
    )
    expect(empty.rows).toHaveLength(1)
  })
})

describe("histogramOptions", () => {
  it("keeps the old behaviour as defaults", () => {
    expect(histogramOptions({})).toMatchObject({
      stack: "normal",
      combine: false,
      legend: "bottom",
      tooltip: "all",
      tooltipHideZeros: true,
    })
  })
})
