import { describe, expect, it } from "vitest"

import { parseAnchor } from "@/conversation/anchor"

import { hitTest as alertAt } from "./alertlist/anchors"
import { hitTest as barAt } from "./barchart/anchors"
import type { Layout } from "./barchart/barchart-model"
import { hitTest as heatmapAt } from "./heatmap/anchors"
import { hitTest as histogramAt } from "./histogram/anchors"
import { hitTest as logsAt } from "./logs/anchors"
import { hitTest as stateAt } from "./state-timeline/anchors"
import { hitTest as tableAt } from "./table/anchors"
import { hitTest as timeseriesAt, rangeTest } from "./timeseries/anchors"
import { hitTest as valueAt } from "./value-anchors"

const body = { width: 400, height: 200 }
const point = { x: 10, y: 10 }

const manyLabels = Object.fromEntries(
  Array.from({ length: 30 }, (_, index) => [
    `${index}-${"k".repeat(250)}`,
    "v".repeat(250),
  ])
)

describe("hit tests on empty or zero-size input", () => {
  it("barchart returns null with no categories or no plot", () => {
    const model = {
      categories: [],
      series: [],
      bars: [],
    } as unknown as Parameters<typeof barAt>[0]["model"]
    const layout = (count: number, width: number) =>
      ({
        body,
        origin: { x: 0, y: 0 },
        layout: {
          orientation: "vertical",
          plot: { width, height: 100 },
          count,
        } as Layout,
        model,
      }) as Parameters<typeof barAt>[0]
    expect(barAt(layout(0, 200), point)).toBeNull()
    expect(barAt(layout(2, 0), { x: 0, y: 10 })).toBeNull()
  })

  it("heatmap returns null for a plot with no width or no height", () => {
    const model = {
      xs: [1000, 2000],
      rows: [{ label: "a" }],
      cells: [[1, 2]],
    } as unknown as Parameters<typeof heatmapAt>[0]["model"]
    const surface = (plot: { width: number; height: number }) => ({
      body,
      origin: { x: 0, y: 0 },
      plot,
      model,
      format: String,
    })
    expect(heatmapAt(surface({ width: 0, height: 50 }), { x: 0, y: 10 })).toBe(
      null
    )
    expect(heatmapAt(surface({ width: 50, height: 0 }), { x: 10, y: 0 })).toBe(
      null
    )
  })

  it("histogram returns null for a plot with no width", () => {
    expect(
      histogramAt(
        {
          body,
          origin: { x: 0, y: 0 },
          plot: { width: 0, height: 50 },
          edges: [0, 1, 2],
          format: String,
        },
        { x: 0, y: 10 }
      )
    ).toBeNull()
  })

  it("state timeline returns null for a plot with no width", () => {
    expect(
      stateAt(
        {
          body,
          plot: { x: 0, y: 0, width: 0, height: 50 },
          range: { from: 0, to: 1000 },
          rowHeight: 20,
          rows: [{ name: "a", segments: [] }],
        },
        { x: 0, y: 10 }
      )
    ).toBeNull()
  })

  describe("timeseries", () => {
    const series = [
      {
        name: "up",
        labels: { job: "api" },
        ys: [10],
        values: [1],
        format: String,
      },
    ]
    const flat = {
      body,
      plot: { x: 0, y: 0, width: 0, height: 100 },
      xMin: 0,
      xMax: 100,
      xs: [0],
      series,
    }
    const empty = {
      ...flat,
      plot: { x: 0, y: 0, width: 300, height: 100 },
      xMin: Number.NaN,
      xMax: Number.NaN,
      xs: [],
      series: [],
    }

    it("returns null for a plot with no width", () => {
      expect(timeseriesAt(flat, { x: 0, y: 10 })).toBeNull()
      expect(rangeTest(flat, { x: 0, y: 10 }, { x: 0, y: 20 })).toBeNull()
    })

    it("returns null when there is no time axis", () => {
      expect(timeseriesAt(empty, point)).toBeNull()
      expect(rangeTest(empty, point, { x: 50, y: 20 })).toBeNull()
    })
  })

  it("logs leave out a time that is not a date", () => {
    const anchor = logsAt(
      {
        body,
        lines: [
          {
            rect: { x: 0, y: 0, width: 400, height: 20 },
            entry: {
              key: "a",
              time: Number.NaN,
              message: "",
              labels: {},
            },
          },
        ],
      },
      point
    )
    expect(anchor).toEqual({
      kind: "log_line",
      pos: expect.anything(),
      id: "a",
    })
  })
})

describe("anchors stay inside what the server accepts", () => {
  it("timeseries with many long labels", () => {
    const layout = {
      body,
      plot: { x: 0, y: 0, width: 300, height: 100 },
      xMin: 0,
      xMax: 100,
      xs: [0, 100],
      series: [
        {
          name: "up",
          labels: { ...manyLabels, "": "empty name" },
          ys: [10, 10],
          values: [1, 1],
          format: String,
        },
      ],
    }
    const anchor = timeseriesAt(layout, { x: 1, y: 10 })
    expect(anchor).not.toBeNull()
    expect(() => parseAnchor(anchor)).not.toThrow()
    const line = timeseriesAt(layout, { x: 50, y: 10 })
    expect(() => parseAnchor(line)).not.toThrow()
  })

  it("table row with many dimension columns", () => {
    const columns = Array.from({ length: 30 }, (_, index) => ({
      name: `${index}-${"c".repeat(250)}`,
      left: index * 10,
      width: 10,
      dimension: true,
    }))
    const anchor = tableAt(
      {
        body,
        gridBottom: body.height,
        header: null,
        columns,
        rows: [
          {
            top: 0,
            height: 20,
            cells: columns.map(() => "v".repeat(250)),
          },
        ],
      },
      point
    )
    expect(anchor).not.toBeNull()
    expect(() => parseAnchor(anchor)).not.toThrow()
  })

  it("alert row with many labels", () => {
    const anchor = alertAt(
      {
        body,
        rows: [
          {
            rect: { x: 0, y: 0, width: 400, height: 20 },
            alert: { name: "A", labels: Object.entries(manyLabels) },
          },
        ],
      },
      point
    )
    expect(() => parseAnchor(anchor)).not.toThrow()
  })

  it("log line with many labels and a long message", () => {
    const anchor = logsAt(
      {
        body,
        lines: [
          {
            rect: { x: 0, y: 0, width: 400, height: 20 },
            entry: {
              key: "k".repeat(300),
              time: 1000,
              message: "m".repeat(400),
              labels: manyLabels,
            },
          },
        ],
      },
      point
    )
    expect(() => parseAnchor(anchor)).not.toThrow()
  })

  it("value with many labels", () => {
    const anchor = valueAt(
      {
        body,
        targets: [
          {
            rect: { x: 0, y: 0, width: 400, height: 200 },
            name: "n",
            labels: manyLabels,
            text: "t".repeat(300),
          },
        ],
      },
      point
    )
    expect(() => parseAnchor(anchor)).not.toThrow()
  })

  it("text of four byte characters that would encode past the limit", () => {
    const anchor = valueAt(
      {
        body,
        targets: [
          {
            rect: { x: 0, y: 0, width: 400, height: 200 },
            name: "😀".repeat(300),
            labels: undefined,
            text: "😀".repeat(300),
          },
        ],
      },
      point
    )
    if (anchor !== null) expect(() => parseAnchor(anchor)).not.toThrow()
  })
})
