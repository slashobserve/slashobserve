import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import { linearScale } from "../value-axis"
import {
  axisTicks,
  barOptions,
  barRect,
  barSource,
  barchartModel,
  categoryBand,
  labelSpot,
  labelStep,
  labelStride,
  maxCategories,
  noCategoriesReason,
  orientationOf,
  rotatedExtent,
  roundedBarPath,
  thresholdMarks,
  tooltipIndexes,
  truncateLabel,
  truncatedNote,
} from "./barchart-model"

const wide = { width: 600, height: 300 }

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Bars",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "barchart",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function frame(
  fields: PreparedFrame["fields"],
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return { ref: "A", fields, ...extra }
}

function text(name: string, values: string[]): PreparedFrame["fields"][number] {
  return { name, type: "string", values }
}

function numbers(
  name: string,
  values: (number | null)[]
): PreparedFrame["fields"][number] {
  return { name, type: "number", values }
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {},
  size = wide,
  hidden: ReadonlySet<string> = new Set(),
  zone: string | undefined = undefined
) {
  const merged = displayWithDefaults("barchart", display)
  const built = panel(extra)
  const source = barSource(
    built,
    frames,
    createFieldResolver(built),
    merged,
    zone
  )
  return barchartModel(source, merged, size, hidden)
}

describe("barchartModel categories", () => {
  it("uses the first text field, every number field is a series", () => {
    const model = build([
      frame([
        numbers("a", [1, 2]),
        text("route", ["/x", "/y"]),
        numbers("b", [3, 4]),
      ]),
    ])
    expect(model.categories.map((category) => category.label)).toEqual([
      "/x",
      "/y",
    ])
    expect(model.series.map((series) => series.name)).toEqual(["a", "b"])
    expect(model.bars[1].map((bar) => bar.value)).toEqual([2, 4])
    expect(model.reason).toBeNull()
  })

  it("keeps working on a time column longer than a function call can take as arguments", () => {
    const count = 300_000
    const model = build([
      frame([
        {
          name: "Time",
          type: "time",
          values: Array.from({ length: count }, (_, i) => i * 60000),
        },
        numbers(
          "a",
          Array.from({ length: count }, () => 1)
        ),
      ]),
    ])
    expect(model.categories).toHaveLength(maxCategories)
  })

  it("falls back to the time field when there is no text field", () => {
    const model = build([
      frame([
        { name: "Time", type: "time", values: [0, 60000] },
        numbers("v", [1, 2]),
      ]),
    ])
    expect(model.categories).toHaveLength(2)
    expect(model.categories[0].title).toMatch(/^\d{4}-\d{2}-\d{2} /)
    expect(model.categories[0].label).toMatch(/^\d{2}:\d{2}:\d{2}$/)
    expect(model.series.map((series) => series.name)).toEqual(["v"])
  })

  it("gives each series of an instant result its own category", () => {
    const instant = (pod: string, value: number) =>
      frame(
        [
          { name: "Time", type: "time", values: [60000] },
          { name: "Value", type: "number", values: [value], seriesName: pod },
        ],
        { labels: { pod } }
      )
    const model = build([instant("checkout-0", 3), instant("checkout-1", 5)])
    expect(model.categories.map((category) => category.label)).toEqual([
      "checkout-0",
      "checkout-1",
    ])
    expect(model.series.map((series) => series.name)).toEqual(["Value"])
    expect(model.bars.map((row) => row.map((bar) => bar.value))).toEqual([
      [3],
      [5],
    ])
  })

  it("names default-named series by the labels that differ", () => {
    const series = (pod: string, values: number[]) =>
      frame(
        [
          { name: "Time", type: "time", values: values.map((_, i) => i) },
          {
            name: "Value",
            type: "number",
            values,
            seriesName: `up{job="api", pod="${pod}"}`,
          },
        ],
        { name: "up", labels: { job: "api", pod } }
      )
    const instant = build([
      series("checkout-0", [3]),
      series("checkout-1", [5]),
    ])
    expect(instant.categories.map((category) => category.label)).toEqual([
      "checkout-0",
      "checkout-1",
    ])
    const range = build([
      series("checkout-0", [1, 2]),
      series("checkout-1", [3, 4]),
    ])
    expect(range.series.map((entry) => entry.name)).toEqual([
      "checkout-0",
      "checkout-1",
    ])
  })

  it("uses the text field of one row frames as one bar per frame", () => {
    const row = (route: string, value: number) =>
      frame([text("route", [route]), numbers("hits", [value])])
    const model = build([row("/a", 1), row("/b", 2), row("/c", 4)])
    expect(model.categories.map((category) => category.label)).toEqual([
      "/a",
      "/b",
      "/c",
    ])
    expect(model.series.map((series) => series.name)).toEqual(["hits"])
    expect(model.bars.map((row) => row.map((bar) => bar.value))).toEqual([
      [1],
      [2],
      [4],
    ])
  })

  it("keeps the time axis for instant results when xField names the time field", () => {
    const model = build(
      [
        frame([{ name: "Time", type: "time", values: [0] }, numbers("v", [1])]),
        frame([{ name: "Time", type: "time", values: [0] }, numbers("w", [2])]),
      ],
      {},
      { xField: "Time" }
    )
    expect(model.categories).toHaveLength(1)
    expect(model.series.map((series) => series.name)).toEqual(["v", "w"])
  })

  it("colors an instant bar by its own series override", () => {
    const overridden = panel({
      overrides: [{ match: { series: "y" }, color: "critical" }],
    } as Partial<DataPanel>)
    const source = barSource(
      overridden,
      [
        frame(
          [
            { name: "Time", type: "time", values: [0] },
            { name: "Value", type: "number", values: [1], seriesName: "x" },
          ],
          { labels: { pod: "x" } }
        ),
        frame(
          [
            { name: "Time", type: "time", values: [0] },
            { name: "Value", type: "number", values: [2], seriesName: "y" },
          ],
          { labels: { pod: "y" } }
        ),
      ],
      createFieldResolver(overridden),
      displayWithDefaults("barchart", {}),
      undefined
    )
    expect([...(source.columns[0].colors ?? new Map()).entries()]).toEqual([
      [1, "var(--threshold-critical)"],
    ])
  })

  it("labels time categories in the given zone", () => {
    const model = build(
      [
        frame([
          { name: "Time", type: "time", values: [0, 60000] },
          numbers("v", [1, 2]),
        ]),
      ],
      {},
      {},
      wide,
      new Set(),
      "Asia/Kolkata"
    )
    expect(model.categories.map((category) => category.title)).toEqual([
      "1970-01-01 05:30:00",
      "1970-01-01 05:31:00",
    ])
    expect(model.categories[0].label).toBe("05:30:00")
  })

  it("prefers xField, matched by field name, over the text field", () => {
    const model = build(
      [frame([text("route", ["/x"]), text("pod", ["p1"]), numbers("v", [1])])],
      {},
      { xField: "pod" }
    )
    expect(model.categories.map((category) => category.label)).toEqual(["p1"])
  })

  it("joins two frames on the x value in first-seen order", () => {
    const model = build([
      frame([text("route", ["/a", "/b"]), numbers("one", [1, 2])], {
        ref: "A",
      }),
      frame([text("route", ["/b", "/c"]), numbers("two", [20, 30])], {
        ref: "B",
      }),
    ])
    expect(model.categories.map((category) => category.label)).toEqual([
      "/a",
      "/b",
      "/c",
    ])
    expect(model.bars.map((row) => row.map((bar) => bar.value))).toEqual([
      [1, null],
      [2, 20],
      [null, 30],
    ])
  })

  it("skips frames without the x field", () => {
    const model = build(
      [
        frame([text("route", ["/a"]), numbers("one", [1])]),
        frame([numbers("loose", [5])], { ref: "B" }),
      ],
      {},
      { xField: "route" }
    )
    expect(model.series.map((series) => series.name)).toEqual(["one"])
  })

  it("leaves hidden fields out of the series", () => {
    const model = build(
      [frame([text("r", ["/a"]), numbers("keep", [1]), numbers("drop", [2])])],
      {
        overrides: [
          {
            match: { field: "drop" },
            hidden: true,
          },
        ],
      } as Partial<DataPanel>
    )
    expect(model.series.map((series) => series.name)).toEqual(["keep"])
  })

  it("explains the missing x field", () => {
    const model = build([frame([numbers("a", [1, 2])])])
    expect(model.reason).toBe(noCategoriesReason)
    expect(model.categories).toEqual([])
  })

  it("caps categories and says so", () => {
    const labels = Array.from({ length: maxCategories + 5 }, (_, i) => `c${i}`)
    const started = performance.now()
    const model = build([
      frame([
        text("r", labels),
        numbers(
          "v",
          labels.map((_, i) => i)
        ),
      ]),
    ])
    expect(performance.now() - started).toBeLessThan(100)
    expect(model.categories).toHaveLength(maxCategories)
    expect(model.bars).toHaveLength(maxCategories)
    expect(model.note).toBe(truncatedNote)
  })
})

describe("barchartModel stacking", () => {
  const frames = [
    frame([
      text("r", ["/a", "/b"]),
      numbers("one", [1, -2]),
      numbers("two", [3, 4]),
    ]),
  ]

  it("draws unstacked bars from zero over a range that spans zero", () => {
    const model = build(frames)
    expect(model.slotCount).toBe(2)
    expect(model.bars[0].map(({ start, end }) => [start, end])).toEqual([
      [0, 1],
      [0, 3],
    ])
    expect([model.min, model.max]).toEqual([-2, 4])
  })

  it("stacks fields of the same mode in one slot", () => {
    const model = build(frames, { stack: "normal" } as Partial<DataPanel>)
    expect(model.slotCount).toBe(1)
    expect(model.bars[0].map(({ start, end }) => [start, end])).toEqual([
      [0, 1],
      [1, 4],
    ])
  })

  it("stacks negative values downward apart from positive ones", () => {
    const model = build(
      [frame([text("r", ["/a"]), numbers("p", [2]), numbers("n", [-3])])],
      { stack: "normal" } as Partial<DataPanel>
    )
    expect(model.bars[0].map(({ start, end }) => [start, end])).toEqual([
      [0, 2],
      [0, -3],
    ])
    expect([model.min, model.max]).toEqual([-3, 2])
  })

  it("normalises percent stacks to 100 per category", () => {
    const model = build(frames, { stack: "percent" } as Partial<DataPanel>)
    expect(model.bars[0].map(({ start, end }) => [start, end])).toEqual([
      [0, 25],
      [25, 100],
    ])
    expect(model.bars[1][1].shown.text).toBe("4")
    expect(model.axis.unit).toBe("percent")
    expect(model.max).toBe(100)
  })

  it("widens the range by a field's min and max", () => {
    const model = build(frames, { min: -10, max: 50 } as Partial<DataPanel>)
    expect([model.min, model.max]).toEqual([-10, 50])
  })

  it("widens an all-zero range by one", () => {
    const model = build([frame([text("r", ["/a"]), numbers("z", [0])])])
    expect([model.min, model.max]).toEqual([0, 1])
  })
})

describe("barchartModel colors", () => {
  it("uses the palette, a fixed color or the threshold color", () => {
    const frames = [frame([text("r", ["/a", "/b"]), numbers("one", [1, 9])])]
    expect(build(frames).bars[0][0].color).toBe("var(--series-1)")
    expect(
      build(frames, { color: "#ff0000" } as Partial<DataPanel>).series[0].color
    ).toBe("#ff0000")
    const thresholds = build(frames, {
      thresholds: [
        { value: null, color: "green" },
        { value: 5, color: "red" },
      ],
    } as Partial<DataPanel>)
    expect(thresholds.bars[0][0].color).not.toBe(thresholds.bars[1][0].color)
  })
})

describe("orientationOf", () => {
  it("follows the aspect on auto and obeys an explicit value", () => {
    expect(orientationOf({ orientation: "auto" }, wide)).toBe("vertical")
    expect(
      orientationOf({ orientation: "auto" }, { width: 200, height: 400 })
    ).toBe("horizontal")
    expect(orientationOf({ orientation: "horizontal" }, wide)).toBe(
      "horizontal"
    )
    expect(
      orientationOf({ orientation: "vertical" }, { width: 1, height: 9 })
    ).toBe("vertical")
  })
})

describe("axisTicks", () => {
  it("snaps to a round step that spans the data", () => {
    expect(axisTicks(0, 7, 5)).toEqual({
      min: 0,
      max: 8,
      ticks: [0, 2, 4, 6, 8],
    })
    expect(axisTicks(-2, 4, 6).ticks).toEqual([-2, -1, 0, 1, 2, 3, 4])
    expect(axisTicks(0, 0.3, 3).ticks).toEqual([0, 0.1, 0.2, 0.3])
  })
})

describe("labelStride", () => {
  it("skips labels that would overlap", () => {
    expect(labelStride(10, 100, 30)).toBe(3)
    expect(labelStride(10, 1000, 30)).toBe(1)
  })
})

describe("barRect", () => {
  const layout = {
    orientation: "vertical" as const,
    plot: { width: 100, height: 100 },
    min: 0,
    max: 10,
    scale: linearScale,
    count: 2,
    slotCount: 2,
    groupWidth: 0.5,
    barWidth: 1,
  }

  it("places grouped columns inside each category slot", () => {
    expect(barRect(layout, 0, 0, { start: 0, end: 5 })).toEqual({
      x: 12.5,
      y: 50,
      width: 12.5,
      height: 50,
    })
    expect(barRect(layout, 1, 1, { start: 0, end: 10 })).toEqual({
      x: 75,
      y: 0,
      width: 12.5,
      height: 100,
    })
  })

  it("draws a negative bar below the zero line", () => {
    const rect = barRect({ ...layout, min: -10 }, 0, 0, { start: 0, end: -5 })
    expect(rect).toMatchObject({ y: 50, height: 25 })
  })

  it("grows horizontal bars from the left", () => {
    const rect = barRect({ ...layout, orientation: "horizontal" }, 0, 0, {
      start: 0,
      end: 5,
    })
    expect(rect).toEqual({ x: 0, y: 12.5, width: 50, height: 12.5 })
  })
})

describe("labelSpot", () => {
  const plot = { width: 200, height: 200 }
  const options = { stacked: false, negative: false }

  it("writes inside a long enough bar and outside a short one", () => {
    const long = { x: 0, y: 100, width: 30, height: 100 }
    const short = { x: 0, y: 190, width: 30, height: 10 }
    expect(labelSpot("auto", long, "vertical", plot, 20, options)).toBe(
      "inside"
    )
    expect(labelSpot("auto", short, "vertical", plot, 20, options)).toBe(
      "outside"
    )
  })

  it("skips automatic text on a bar thinner than 16px or than its text", () => {
    const thin = { x: 0, y: 0, width: 100, height: 12 }
    expect(labelSpot("auto", thin, "horizontal", plot, 20, options)).toBeNull()
    const narrow = { x: 0, y: 100, width: 10, height: 100 }
    expect(labelSpot("auto", narrow, "vertical", plot, 20, options)).toBeNull()
    expect(labelSpot("always", narrow, "vertical", plot, 20, options)).toBe(
      "inside"
    )
  })

  it("never writes when asked not to and never outside a stack", () => {
    const short = { x: 0, y: 190, width: 30, height: 10 }
    expect(labelSpot("never", short, "vertical", plot, 20, options)).toBeNull()
    expect(
      labelSpot("auto", short, "vertical", plot, 20, {
        stacked: true,
        negative: false,
      })
    ).toBeNull()
  })
})

describe("barchartModel legend hiding", () => {
  const frames = [
    frame([text("r", ["/a"]), numbers("one", [1]), numbers("two", [3])]),
  ]

  it("drops a hidden series and restacks the rest", () => {
    const stacked = { stack: "normal" } as Partial<DataPanel>
    const model = build(frames, stacked, {}, wide, new Set(["A:{}#one"]))
    expect(model.series.map((series) => series.name)).toEqual(["two"])
    expect(model.bars[0].map(({ start, end }) => [start, end])).toEqual([
      [0, 3],
    ])
  })
})

describe("barchartModel scales", () => {
  it("draws log bars from the bottom of the decade range", () => {
    const model = build(
      [frame([text("r", ["/a", "/b"]), numbers("v", [5, 500])])],
      { scale: "log" } as Partial<DataPanel>
    )
    expect(model.scale.kind).toBe("log")
    expect(model.min).toBe(1)
    expect(model.max).toBe(1000)
  })

  it("draws symlog bars around zero", () => {
    const model = build(
      [frame([text("r", ["/a", "/b"]), numbers("v", [-50, 500])])],
      { scale: "symlog" } as Partial<DataPanel>
    )
    expect(model.scale.kind).toBe("symlog")
    expect(model.min).toBeLessThan(0)
  })
})

describe("barchartModel fill and border", () => {
  const frames = [frame([text("r", ["/a"]), numbers("v", [1])])]

  it("is opaque with no border unless fill and line width are written", () => {
    const plain = build(frames).series[0]
    expect([plain.fillOpacity, plain.borderWidth]).toEqual([1, 0])
    const written = build(frames, {
      fill: 40,
      lineWidth: 3,
    } as Partial<DataPanel>).series[0]
    expect([written.fillOpacity, written.borderWidth]).toEqual([0.4, 3])
  })
})

describe("barchartModel colorByField", () => {
  it("colors each category by the threshold of the named field", () => {
    const frames = [
      frame([
        text("r", ["/a", "/b"]),
        numbers("v", [1, 1]),
        numbers("level", [1, 9]),
      ]),
    ]
    const model = build(
      frames,
      {
        thresholds: [
          { value: null, color: "green" },
          { value: 5, color: "red" },
        ],
      } as Partial<DataPanel>,
      { colorByField: "level" }
    )
    const first = model.bars[0][0].color
    expect(first).not.toBe(model.bars[1][0].color)
  })
})

describe("tooltipIndexes", () => {
  const frames = [
    frame([
      text("r", ["/a"]),
      numbers("low", [1]),
      numbers("high", [9]),
      numbers("zero", [0]),
    ]),
  ]
  const model = build(frames)
  const hit = { category: 0, bar: 0 }
  const names = (indexes: number[]) =>
    indexes.map((index) => model.series[index].name)

  it("lists only the hovered bar in single mode", () => {
    const options = barOptions({ tooltip: "single" })
    expect(names(tooltipIndexes(model, hit, options))).toEqual(["low"])
  })

  it("sorts the series of the category in all mode", () => {
    const base = { tooltip: "all" } as const
    expect(
      names(
        tooltipIndexes(model, hit, barOptions({ ...base, tooltipSort: "desc" }))
      )
    ).toEqual(["high", "low", "zero"])
    expect(
      names(
        tooltipIndexes(model, hit, barOptions({ ...base, tooltipSort: "asc" }))
      )
    ).toEqual(["zero", "low", "high"])
  })

  it("drops zero rows with tooltipHideZeros", () => {
    const options = barOptions({ tooltip: "all", tooltipHideZeros: true })
    expect(names(tooltipIndexes(model, hit, options))).toEqual(["low", "high"])
  })
})

describe("category labels", () => {
  it("truncates by characters and marks the cut", () => {
    expect(truncateLabel("checkout", 5)).toBe("check\u2026")
    expect(truncateLabel("short", 10)).toBe("short")
    expect(truncateLabel("anything")).toBe("anything")
  })

  it("lets a spacing replace the measured label length", () => {
    expect(labelStep(10, 100, 5, 0)).toBe(1)
    expect(labelStep(10, 100, 5, 30)).toBe(3)
  })

  it("grows the axis with the rotated text", () => {
    expect(rotatedExtent(100, 0, 16)).toBe(16)
    expect(rotatedExtent(100, 90, 16)).toBe(100)
    expect(rotatedExtent(100, -45, 16)).toBe(rotatedExtent(100, 45, 16))
  })
})

describe("roundedBarPath", () => {
  const rect = { x: 0, y: 0, width: 20, height: 40 }

  it("rounds the free end of a positive vertical bar", () => {
    const path = roundedBarPath(rect, "vertical", false, 0.25)
    expect(path).toContain("A5 5")
    expect(path?.startsWith("M0 40")).toBe(true)
  })

  it("draws nothing special without a radius", () => {
    expect(roundedBarPath(rect, "vertical", false, 0)).toBeNull()
  })

  it("keeps the radius within half the thickness and the length", () => {
    expect(roundedBarPath(rect, "vertical", false, 0.5)).toContain("A10 10")
    const stub = { x: 0, y: 0, width: 20, height: 3 }
    expect(roundedBarPath(stub, "vertical", false, 0.5)).toContain("A3 3")
  })
})

describe("categoryBand", () => {
  it("spans the whole category across the other axis", () => {
    const layout = {
      orientation: "vertical" as const,
      plot: { width: 100, height: 50 },
      min: 0,
      max: 1,
      scale: linearScale,
      count: 4,
      slotCount: 1,
      groupWidth: 0.7,
      barWidth: 1,
    }
    expect(categoryBand(layout, 1)).toEqual({
      x: 25,
      y: 0,
      width: 25,
      height: 50,
    })
  })
})

const steps = [
  { color: "green" },
  { value: 50, color: "orange" },
  { value: 80, color: "red" },
]

describe("barchartModel threshold style", () => {
  const frames = [frame([text("r", ["/a", "/b"]), numbers("one", [10, 90])])]
  const styled = (thresholdsStyle?: string, extra: Partial<DataPanel> = {}) =>
    build(frames, {
      thresholds: steps,
      ...(thresholdsStyle ? { thresholdsStyle } : {}),
      ...extra,
    } as Partial<DataPanel>).thresholds

  it("draws lines and bands only for a style that was written", () => {
    expect(styled()).toBeNull()
    expect(styled("off")).toBeNull()
    expect(styled("series")).toBeNull()
    for (const style of [
      "line",
      "dashed",
      "area",
      "line+area",
      "dashed+area",
    ]) {
      expect(styled(style)).toMatchObject({ list: steps, style })
    }
  })

  it("draws nothing without thresholds", () => {
    const model = build(frames, {
      thresholdsStyle: "line",
    } as Partial<DataPanel>)
    expect(model.thresholds).toBeNull()
  })

  it("draws nothing on a percent stack, whose axis is not in value units", () => {
    expect(
      styled("line", { stack: "percent" } as Partial<DataPanel>)
    ).toBeNull()
  })

  it("keeps coloring bars by their threshold in the series style", () => {
    const model = build(frames, {
      thresholds: steps,
      thresholdsStyle: "series",
    } as Partial<DataPanel>)
    expect(model.bars[0][0].color).not.toBe(model.bars[1][0].color)
  })

  it("reads the style of an override on the first series", () => {
    const model = build(frames, {
      thresholds: steps,
      overrides: [{ match: { series: "one" }, thresholdsStyle: "area" }],
    } as Partial<DataPanel>)
    expect(model.thresholds).toMatchObject({ style: "area" })
  })
})

describe("thresholdMarks", () => {
  const marks = (style: string, mode = "absolute", min = 0, max = 100) =>
    thresholdMarks(
      { list: steps, mode, style } as Parameters<typeof thresholdMarks>[0],
      min,
      max
    )

  it("draws a solid or dashed line at every step that has a value", () => {
    const solid = marks("line")
    expect(solid.lines.map(({ at }) => at)).toEqual([50, 80])
    expect(solid.dashed).toBe(false)
    expect(solid.bands).toEqual([])
    expect(marks("dashed").dashed).toBe(true)
  })

  it("fills a band per step between its value and the next, clipped to the axis", () => {
    const area = marks("area", "absolute", 20, 70)
    expect(area.lines).toEqual([])
    expect(area.bands.map(({ from, to }) => [from, to])).toEqual([
      [20, 50],
      [50, 70],
    ])
  })

  it("draws lines and bands together in the combined styles", () => {
    const both = marks("dashed+area")
    expect(both.lines).toHaveLength(2)
    expect(both.bands).toHaveLength(3)
    expect(both.dashed).toBe(true)
  })

  it("leaves out lines beyond the axis and places percentages in the range", () => {
    expect(marks("line", "absolute", 0, 60).lines.map(({ at }) => at)).toEqual([
      50,
    ])
    expect(
      marks("line", "percentage", 0, 200).lines.map(({ at }) => at)
    ).toEqual([100, 160])
  })
})

describe("barOptions", () => {
  it("reads the defaults and the written values", () => {
    const defaults = barOptions({})
    expect(defaults).toMatchObject({
      legend: "bottom",
      tooltip: "single",
      tooltipSort: "none",
      barRadius: 0,
      fullHighlight: false,
      xTickRotation: 0,
      valueSize: 12,
    })
    expect(barOptions({ valueSize: 20, barRadius: 0.3 })).toMatchObject({
      valueSize: 20,
      barRadius: 0.3,
    })
  })
})
