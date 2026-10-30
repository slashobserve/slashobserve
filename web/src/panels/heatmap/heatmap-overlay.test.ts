import { describe, expect, it } from "vitest"

import { displayWithDefaults } from "@/spec"

import type { ExemplarPoint } from "../timeseries/exemplars"
import {
  heatmapLook,
  type HeatmapModel,
  type HeatmapLook,
} from "./heatmap-model"
import {
  axisLook,
  columnEntries,
  exemplarLook,
  exemplarMarks,
  histogramBars,
  markAt,
  maxTooltipRows,
  rowPosition,
  scaleMarker,
  tooltipLook,
} from "./heatmap-overlay"

function model(
  cells: (number | null)[][],
  overrides: Partial<HeatmapModel> = {}
): HeatmapModel {
  const rows = cells.map((_, i) => ({
    label: `${i * 10}`,
    range: `${i * 10} - ${(i + 1) * 10}`,
    from: i * 10,
    to: (i + 1) * 10,
  }))
  return {
    mode: "buckets",
    xs: [0, 1000, 2000, 3000],
    rows,
    cells,
    min: 1,
    max: 9,
    unit: undefined,
    reversed: false,
    ...overrides,
  }
}

function lookOf(display: Record<string, unknown> = {}): HeatmapLook {
  return heatmapLook(displayWithDefaults("heatmap", display), {
    min: 1,
    max: 9,
  })
}

describe("tooltipLook", () => {
  it("shows the hovered cell only by default", () => {
    expect(tooltipLook(displayWithDefaults("heatmap", {}))).toEqual({
      mode: "single",
      histogram: false,
      colorScale: false,
      maxWidth: undefined,
      maxHeight: undefined,
    })
  })

  it("reads the mode, extras and size limits", () => {
    expect(
      tooltipLook({
        tooltip: "all",
        tooltipHistogram: true,
        tooltipColorScale: true,
        tooltipMaxWidth: 300,
        tooltipMaxHeight: 200,
      })
    ).toEqual({
      mode: "all",
      histogram: true,
      colorScale: true,
      maxWidth: 300,
      maxHeight: 200,
    })
    expect(tooltipLook({ tooltip: "hidden" }).mode).toBe("hidden")
  })
})

describe("columnEntries", () => {
  it("lists the visible cells of a column from the top row down", () => {
    const m = model([[1], [0], [3], [null], [5]])
    const { entries, more } = columnEntries(m, lookOf(), 0, 2)
    expect(entries.map(({ row, value }) => [row, value])).toEqual([
      [4, 5],
      [2, 3],
      [0, 1],
    ])
    expect(entries[0].range).toBe("40 - 50")
    expect(more).toBe(0)
  })

  it("honours hideAbove", () => {
    const m = model([[1], [4], [8]])
    const { entries } = columnEntries(m, lookOf({ hideAbove: 8 }), 0, 0)
    expect(entries.map(({ value }) => value)).toEqual([4, 1])
  })

  it("keeps the hovered row inside the rows it cuts to", () => {
    const m = model(Array.from({ length: 30 }, (_, i) => [i + 1]))
    const { entries, more } = columnEntries(m, lookOf(), 0, 3)
    expect(entries).toHaveLength(maxTooltipRows)
    expect(entries.map(({ row }) => row)).toContain(3)
    expect(more).toBe(20)
    const top = columnEntries(m, lookOf(), 0, 29)
    expect(top.entries[0].row).toBe(29)
  })
})

describe("histogramBars", () => {
  it("scales a column to its largest visible cell, top row first", () => {
    const m = model([[2], [4], [0], [1]])
    expect(histogramBars(m, lookOf(), 0)).toEqual([
      { row: 3, share: 0.25 },
      { row: 2, share: 0 },
      { row: 1, share: 1 },
      { row: 0, share: 0.5 },
    ])
  })

  it("has no bars for a column without visible cells", () => {
    expect(histogramBars(model([[0], [null]]), lookOf(), 0)).toEqual([])
  })
})

describe("scaleMarker", () => {
  it("places a count between the ends of the color range", () => {
    const look = lookOf()
    expect(scaleMarker(look, 1)).toBe(0)
    expect(scaleMarker(look, 5)).toBe(0.5)
    expect(scaleMarker(look, 9)).toBe(1)
    expect(scaleMarker(look, 100)).toBe(1)
  })
})

describe("rowPosition", () => {
  const m = model([[1], [1], [1]])

  it("finds the row of a value with its place inside the row", () => {
    expect(rowPosition(m, 5)).toBe(0.5)
    expect(rowPosition(m, 15)).toBe(1.5)
    expect(rowPosition(m, 30)).toBe(3)
  })

  it("is null outside every row", () => {
    expect(rowPosition(m, -1)).toBeNull()
    expect(rowPosition(m, 31)).toBeNull()
  })

  it("mirrors the place inside a reversed row", () => {
    expect(rowPosition({ ...m, reversed: true }, 12.5)).toBe(1.75)
  })

  it("puts a value in the middle of a row with an open bound", () => {
    const open = model([[1]], {
      rows: [{ label: "10", range: "10 and above", from: 10 }],
    })
    expect(rowPosition(open, 500)).toBe(0.5)
  })
})

describe("exemplarMarks", () => {
  const m = model([[1], [1]])
  const point = (seconds: number, y: number): ExemplarPoint => ({
    x: seconds,
    y,
    scale: "y",
    labels: [["trace_id", "abc"]],
  })

  it("places a point by its time inside its column and its value inside its row", () => {
    const [mark] = exemplarMarks(m, [point(1.5, 5)], 400, 100)
    expect(mark.x).toBe(150)
    expect(mark.y).toBe(75)
    expect(mark.value).toBe(5)
    expect(mark.labels).toEqual([["trace_id", "abc"]])
  })

  it("leaves out points outside the time range or every row", () => {
    expect(exemplarMarks(m, [point(-1, 5), point(4.5, 5)], 400, 100)).toEqual(
      []
    )
    expect(exemplarMarks(m, [point(1, 99)], 400, 100)).toEqual([])
  })

  it("draws none for ordinal rows", () => {
    expect(
      exemplarMarks({ ...m, mode: "ordinal" }, [point(1, 5)], 400, 100)
    ).toEqual([])
  })

  it("puts a point of a single column at its left edge", () => {
    const single = model([[1]], { xs: [5000] })
    const [mark] = exemplarMarks(single, [point(5, 5)], 100, 50)
    expect(mark.x).toBe(0)
  })
})

describe("markAt", () => {
  const marks = [
    { x: 10, y: 10, value: 1, labels: [] },
    { x: 14, y: 10, value: 2, labels: [] },
  ]

  it("finds the nearest mark within 6 pixels", () => {
    expect(markAt(marks, 13, 10)?.value).toBe(2)
    expect(markAt(marks, 4, 10)?.value).toBe(1)
  })

  it("finds nothing farther away", () => {
    expect(markAt(marks, 30, 10)).toBeNull()
    expect(markAt(marks, 10, 17)).toBeNull()
  })
})

describe("axisLook", () => {
  it("defaults to the left with automatic width and no label", () => {
    expect(axisLook(displayWithDefaults("heatmap", {}))).toEqual({
      side: "left",
      width: undefined,
      label: undefined,
    })
  })

  it("reads side, width and label", () => {
    expect(
      axisLook({ yAxis: "right", yAxisWidth: 80, yAxisLabel: "Latency" })
    ).toEqual({ side: "right", width: 80, label: "Latency" })
    expect(axisLook({ yAxis: "hidden" }).side).toBe("hidden")
  })
})

describe("exemplarLook", () => {
  it("draws exemplars in the foreground color by default", () => {
    expect(exemplarLook(displayWithDefaults("heatmap", {}))).toEqual({
      shown: true,
      color: "var(--foreground)",
    })
  })

  it("can be turned off and colored by a semantic name", () => {
    expect(exemplarLook({ exemplars: false }).shown).toBe(false)
    expect(exemplarLook({ exemplarColor: "critical" }).color).toBe(
      "var(--threshold-critical)"
    )
  })
})
