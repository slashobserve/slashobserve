import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  labelPlacement,
  legendCells,
  lineOffsets,
  percentText,
  pieEntries,
  pieModel,
  pieOptions,
  sliceLabelLines,
  slicePath,
  summaryLabel,
  tooltipRows,
} from "./pie-model"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Pie",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "piechart",
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

function entriesOf(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  return pieEntries(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("piechart", display)
  )
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  return pieModel(entriesOf(frames, extra, display))
}

describe("pieModel", () => {
  it("shares the circle by value and sums to 100 percent", () => {
    const { slices, total } = build([
      series("a", [1, 1]),
      series("b", [3]),
      series("c", [6]),
    ])
    expect(total).toBe(10)
    expect(slices.map((slice) => slice.percent)).toEqual([10, 30, 60])
    expect(slices.reduce((sum, slice) => sum + slice.percent, 0)).toBeCloseTo(
      100
    )
    expect(slices[0].startAngle).toBe(0)
    expect(slices[2].endAngle).toBeCloseTo(Math.PI * 2)
    expect(slices[1].startAngle).toBe(slices[0].endAngle)
  })

  it("reduces each field by display.reduce", () => {
    const frames = [series("a", [1, 2, 3]), series("b", [10, 20, null])]
    expect(build(frames).slices.map((slice) => slice.value)).toEqual([3, 20])
    expect(
      build(frames, {}, { reduce: "sum" }).slices.map((slice) => slice.value)
    ).toEqual([6, 30])
  })

  it("leaves null, zero and negative values out of the pie but keeps them as entries", () => {
    const frames = [
      series("a", [4]),
      series("b", [null]),
      series("c", [0]),
      series("d", [-2]),
    ]
    const entries = entriesOf(frames)
    const { slices, total, percents } = pieModel(entries)
    expect(total).toBe(4)
    expect(slices.map((slice) => slice.name)).toEqual(["a"])
    expect(entries.map((item) => item.name)).toEqual(["a", "b", "c", "d"])
    expect(entries.map((item) => item.text)).toEqual(["4", "-", "0", "-2"])
    expect(entries.map((item) => item.value)).toEqual([4, null, 0, -2])
    expect([...percents.values()]).toEqual([100, null, 0, 0])
  })

  it("draws nothing when no value is positive", () => {
    const { slices, total } = build([series("a", [0]), series("b", [null])])
    expect(total).toBe(0)
    expect(slices).toEqual([])
  })

  it("skips hidden fields and non-number fields", () => {
    const frame: PreparedFrame = {
      ...series("a", [1]),
      fields: [
        { name: "Time", type: "time", values: [0] },
        { name: "Label", type: "string", values: ["x"] },
        { name: "Value", type: "number", values: [1] },
      ],
    }
    const hidden = build([frame, series("b", [2])], {
      overrides: [{ match: { series: "b" }, hidden: true }],
    })
    expect(hidden.slices).toHaveLength(1)
    expect(hidden.slices[0].value).toBe(1)
  })

  it("keeps palette colors in field order and honors a field color", () => {
    const frames = [series("a", [1]), series("b", [null]), series("c", [2])]
    const { slices } = build(frames, { color: undefined })
    expect(slices.map((slice) => slice.color)).toEqual([
      "var(--series-1)",
      "var(--series-3)",
    ])
    expect(entriesOf(frames)[1].color).toBe("var(--series-2)")
    const fixed = build([series("a", [1])], { color: "#ff0000" })
    expect(fixed.slices[0].color).toBe("#ff0000")
  })

  it("formats values with the unit and one shared decimal count", () => {
    const { slices } = build(
      [series("a", [2.5]), series("b", [2.45]), series("c", [10])],
      { unit: "reqps" }
    )
    expect(slices.map((slice) => slice.text)).toEqual([
      "2.50 req/s",
      "2.45 req/s",
      "10.00 req/s",
    ])
  })

  it("keeps duplicate series apart by key", () => {
    const { slices } = build([series("a", [1]), series("a", [2])])
    expect(new Set(slices.map((slice) => slice.key)).size).toBe(2)
  })
})

describe("labels", () => {
  it("lists the top five slices with name, value and percent", () => {
    const { slices } = build(
      Array.from({ length: 7 }, (_, i) => series(`s${i}`, [1]))
    )
    const label = summaryLabel(slices)
    expect(label.split(", ")).toHaveLength(5)
    expect(label.startsWith("s0 1 (14.3%)")).toBe(true)
  })

  it("writes percent with one decimal", () => {
    expect(percentText(33.333)).toBe("33.3%")
    expect(percentText(100)).toBe("100.0%")
  })
})

describe("pieOptions", () => {
  it("falls back to what the pie always drew", () => {
    expect(pieOptions({})).toEqual({
      pieType: "pie",
      legend: "right",
      sliceLabels: [],
      titleSize: 12,
      valueSize: 12,
      legendColumns: ["value", "percent"],
      legendMode: "list",
      legendSortBy: undefined,
      legendSortDesc: false,
      legendWidth: undefined,
      tooltip: "single",
      tooltipSort: "none",
      tooltipHideZeros: false,
      tooltipMaxWidth: undefined,
      tooltipMaxHeight: undefined,
    })
  })

  it("reads every pie key", () => {
    const options = pieOptions({
      pieType: "donut",
      legend: "bottom",
      sliceLabels: ["percent", "name"],
      titleSize: 14,
      valueSize: 20,
      legendShow: ["percent"],
      legendMode: "table",
      legendSortBy: "value",
      legendSortDesc: true,
      legendWidth: 200,
      tooltip: "all",
      tooltipSort: "asc",
      tooltipHideZeros: true,
      tooltipMaxWidth: 300,
      tooltipMaxHeight: 150,
    })
    expect(options).toEqual({
      pieType: "donut",
      legend: "bottom",
      sliceLabels: ["name", "percent"],
      titleSize: 14,
      valueSize: 20,
      legendColumns: ["percent"],
      legendMode: "table",
      legendSortBy: "value",
      legendSortDesc: true,
      legendWidth: 200,
      tooltip: "all",
      tooltipSort: "asc",
      tooltipHideZeros: true,
      tooltipMaxWidth: 300,
      tooltipMaxHeight: 150,
    })
  })

  it("keeps an empty legend value list empty", () => {
    expect(pieOptions({ legendShow: [] }).legendColumns).toEqual([])
  })
})

describe("legend toggling", () => {
  it("shares the circle among the series that stay shown", () => {
    const entries = entriesOf([
      series("a", [1]),
      series("b", [3]),
      series("c", [6]),
    ])
    const hiddenKey = entries[1].key
    const { slices, total, percents } = pieModel(entries, new Set([hiddenKey]))
    expect(total).toBe(7)
    expect(slices.map((slice) => slice.percent)).toEqual([
      (1 / 7) * 100,
      (6 / 7) * 100,
    ])
    expect(percents.get(hiddenKey)).toBeCloseTo((3 / 10) * 100)
  })
})

describe("legendCells", () => {
  it("writes the value and percent columns in the order given", () => {
    const entries = entriesOf([series("a", [1]), series("b", [null])])
    const { percents } = pieModel(entries)
    expect(legendCells(entries, percents, ["value", "percent"])).toEqual([
      ["1", "100.0%"],
      ["-", ""],
    ])
    expect(legendCells(entries, percents, ["percent"])).toEqual([
      ["100.0%"],
      [""],
    ])
    expect(legendCells(entries, percents, [])).toEqual([[], []])
  })
})

describe("series flags", () => {
  it("marks series hidden from the legend or the tooltip by their field props", () => {
    const entries = entriesOf([series("a", [1]), series("b", [2])], {
      overrides: [
        { match: { series: "b" }, hideInLegend: true, hideInTooltip: true },
      ],
    })
    expect(entries.map((entry) => entry.hideInLegend)).toEqual([false, true])
    expect(entries.map((entry) => entry.hideInTooltip)).toEqual([false, true])
  })
})

describe("tooltipRows", () => {
  const entries = entriesOf([
    series("a", [2]),
    series("b", [0]),
    series("c", [5]),
    series("d", [null]),
  ])
  const { percents } = pieModel(entries)
  const base = {
    tooltip: "all",
    tooltipSort: "none",
    tooltipHideZeros: false,
  } as const

  it("shows nothing when hidden or when nothing is hovered", () => {
    expect(
      tooltipRows(entries, percents, entries[0].key, {
        ...base,
        tooltip: "hidden",
      })
    ).toEqual([])
    expect(tooltipRows(entries, percents, null, base)).toEqual([])
  })

  it("lists only the hovered slice in single mode", () => {
    const rows = tooltipRows(entries, percents, entries[2].key, {
      ...base,
      tooltip: "single",
    })
    expect(rows.map((row) => row.name)).toEqual(["c"])
    expect(rows[0]).toMatchObject({ text: "5", percent: "71.4%", active: true })
  })

  it("lists every series in all mode with the hovered one marked", () => {
    const rows = tooltipRows(entries, percents, entries[0].key, base)
    expect(rows.map((row) => row.name)).toEqual(["a", "b", "c", "d"])
    expect(rows.map((row) => row.active)).toEqual([true, false, false, false])
  })

  it("sorts by value with empty values last", () => {
    const names = (tooltipSort: "asc" | "desc") =>
      tooltipRows(entries, percents, entries[0].key, {
        ...base,
        tooltipSort,
      }).map((row) => row.name)
    expect(names("asc")).toEqual(["b", "a", "c", "d"])
    expect(names("desc")).toEqual(["c", "a", "b", "d"])
  })

  it("drops zero rows with tooltipHideZeros but keeps empty ones", () => {
    const rows = tooltipRows(entries, percents, entries[0].key, {
      ...base,
      tooltipHideZeros: true,
    })
    expect(rows.map((row) => row.name)).toEqual(["a", "c", "d"])
  })

  it("leaves out series hidden from the tooltip or from the pie", () => {
    const flagged = entries.map((entry) =>
      entry.name === "c" ? { ...entry, hideInTooltip: true } : entry
    )
    const rows = tooltipRows(
      flagged,
      percents,
      entries[0].key,
      base,
      new Set([entries[3].key])
    )
    expect(rows.map((row) => row.name)).toEqual(["a", "b"])
  })

  it("shows nothing for a hovered series hidden from the tooltip", () => {
    const flagged = entries.map((entry, index) =>
      index === 0 ? { ...entry, hideInTooltip: true } : entry
    )
    expect(tooltipRows(flagged, percents, entries[0].key, base)).toEqual([])
  })
})

describe("slice labels", () => {
  const slice = {
    name: "api",
    text: "1.5 GB",
    percent: 25,
    startAngle: 0,
    endAngle: Math.PI / 2,
  } as Parameters<typeof sliceLabelLines>[0]
  const sizes = { title: 14, value: 10 }

  it("writes the chosen parts, each part at its own size", () => {
    expect(sliceLabelLines(slice, ["name", "value", "percent"], sizes)).toEqual(
      [
        { text: "api", size: 14 },
        { text: "1.5 GB (25.0%)", size: 10 },
      ]
    )
    expect(sliceLabelLines(slice, ["percent"], sizes)).toEqual([
      { text: "25.0%", size: 10 },
    ])
    expect(sliceLabelLines(slice, ["name"], sizes)).toEqual([
      { text: "api", size: 14 },
    ])
    expect(sliceLabelLines(slice, [], sizes)).toEqual([])
  })

  it("centers the lines on the anchor", () => {
    expect(
      lineOffsets([
        { text: "a", size: 16 },
        { text: "b", size: 16 },
      ])
    ).toEqual([-10, 10])
  })

  it("places a label at the middle of a pie slice and checks it fits", () => {
    const placed = labelPlacement(slice, [{ text: "api", size: 12 }], 100, 0)
    expect(placed.x).toBeCloseTo(62 * Math.sin(Math.PI / 4))
    expect(placed.y).toBeCloseTo(-62 * Math.cos(Math.PI / 4))
    expect(placed.fits).toBe(true)
  })

  it("drops a label that is wider than its slice", () => {
    const thin = { startAngle: 0, endAngle: 0.1 }
    const lines = [{ text: "a long service name", size: 12 }]
    expect(labelPlacement(thin, lines, 100, 0).fits).toBe(false)
  })

  it("drops a label that is deeper than the ring of a donut", () => {
    const lines = [{ text: "a long service name", size: 12 }]
    const right = { startAngle: Math.PI / 4, endAngle: (3 * Math.PI) / 4 }
    expect(labelPlacement(right, lines, 100, 60).fits).toBe(false)
    const top = { startAngle: -Math.PI / 2, endAngle: Math.PI / 2 }
    expect(labelPlacement(top, lines, 100, 60).fits).toBe(true)
  })

  it("never fits a slice without lines", () => {
    expect(labelPlacement(slice, [], 100, 0).fits).toBe(false)
  })

  it("lets a single full-circle slice hold its label", () => {
    const full = { startAngle: 0, endAngle: Math.PI * 2 }
    expect(
      labelPlacement(full, [{ text: "everything", size: 12 }], 100, 0).fits
    ).toBe(true)
  })
})

describe("slicePath", () => {
  it("closes a wedge at the center for a pie slice", () => {
    expect(slicePath(50, 50, 40, 0, 0, Math.PI / 2)).toBe(
      "M 50 10 A 40 40 0 0 1 90 50 L 50 50 Z"
    )
  })

  it("sets the large-arc flag past a half turn", () => {
    expect(slicePath(50, 50, 40, 0, 0, Math.PI * 1.5)).toContain(
      "A 40 40 0 1 1"
    )
  })

  it("draws a single slice as a full circle of two arcs", () => {
    const path = slicePath(50, 50, 40, 0, 0, Math.PI * 2)
    expect(path.match(/A /g)).toHaveLength(2)
    expect(path).not.toContain("L ")
    expect(path.match(/M /g)).toHaveLength(1)
  })

  it("cuts the inner radius out of a donut slice", () => {
    expect(slicePath(50, 50, 40, 24, 0, Math.PI / 2)).toBe(
      "M 50 10 A 40 40 0 0 1 90 50 L 74 50 A 24 24 0 0 0 50 26 Z"
    )
  })

  it("draws a single donut slice as two concentric circles", () => {
    const path = slicePath(50, 50, 40, 24, 0, Math.PI * 2)
    expect(path.match(/M /g)).toHaveLength(2)
    expect(path).toContain("A 24 24 0 1 0")
    expect(path).toContain("A 40 40 0 1 1")
  })
})

describe("pieModel value options", () => {
  it("draws one slice per row with allValues", () => {
    const { slices } = build([series("a", [1, 3])], {}, { allValues: true })
    expect(slices.map((slice) => slice.value)).toEqual([1, 3])
    expect(slices.map((slice) => slice.percent)).toEqual([25, 75])
  })

  it("stops at the row limit", () => {
    const { slices } = build(
      [series("a", [1, 2, 3])],
      {},
      { allValues: true, limit: 2 }
    )
    expect(slices).toHaveLength(2)
  })

  it("draws only the field the fields option names", () => {
    const frame = series("a", [1], {
      fields: [
        { name: "Time", type: "time", values: [0] },
        { name: "cpu", type: "number", values: [1] },
        { name: "mem", type: "number", values: [2] },
      ],
    })
    const { slices } = build([frame], {}, { fields: "mem" })
    expect(slices.map((slice) => slice.value)).toEqual([2])
  })
})
