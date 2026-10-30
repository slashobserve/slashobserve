import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  barLimits,
  barLinks,
  barSizes,
  barStyleOf,
  bargaugeModel,
  lcdCellCount,
  lcdLitCells,
  namePlacementOf,
  orientationOf,
  reducerOf,
  showUnfilledOf,
  valueModeOf,
} from "./bargauge-model"

beforeEach(() => {
  vi.stubGlobal("location", { origin: "https://observe.example.com" })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "b1",
    title: "Bars",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "bargauge",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function series(
  values: (number | null)[],
  labels: Record<string, string> = {},
  extra: Partial<PreparedFrame> = {}
): PreparedFrame {
  return {
    ref: "A",
    labels,
    fields: [
      { name: "Time", type: "time", values: values.map((_, i) => i) },
      { name: "Value", type: "number", values },
    ],
    ...extra,
  }
}

const wide = { width: 400, height: 100 }
const tall = { width: 100, height: 400 }

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  const fields = createFieldResolver(panel(extra))
  const merged = displayWithDefaults("bargauge", display)
  return {
    fields,
    display: merged,
    model: bargaugeModel(frames, fields, merged),
  }
}

describe("orientation", () => {
  const orientation = (
    display: Record<string, unknown>,
    size: { width: number; height: number }
  ) => orientationOf(displayWithDefaults("bargauge", display), size)

  it("is horizontal when the body is wider than tall", () => {
    expect(orientation({}, wide)).toBe("horizontal")
  })

  it("is vertical when the body is taller than wide", () => {
    expect(orientation({}, tall)).toBe("vertical")
  })

  it("is vertical when the body is square", () => {
    expect(orientation({}, { width: 200, height: 200 })).toBe("vertical")
  })

  it("follows an explicit orientation over the body's aspect", () => {
    expect(orientation({ orientation: "horizontal" }, tall)).toBe("horizontal")
    expect(orientation({ orientation: "vertical" }, wide)).toBe("vertical")
  })
})

describe("bars", () => {
  it("makes one bar per number field with the series name", () => {
    const { model } = build([
      series([1, 2], { pod: "a" }),
      series([5, 6], { pod: "b" }),
    ])
    expect(model.bars.map((bar) => bar.name)).toEqual([
      '{pod="a"}',
      '{pod="b"}',
    ])
  })

  it("names a bar by its display name", () => {
    const { model } = build([series([1])], { displayName: "Pods" })
    expect(model.bars[0].name).toBe("Pods")
  })

  it("keeps the order of the frames", () => {
    const { model } = build([
      series([1], { pod: "low" }),
      series([9], { pod: "high" }),
    ])
    expect(model.bars.map((bar) => bar.value)).toEqual([1, 9])
  })

  it("skips non-number fields and hidden fields", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "Time", type: "time", values: [1] },
        { name: "host", type: "string", values: ["x"] },
        { name: "shown", type: "number", values: [1] },
        { name: "secret", type: "number", values: [2] },
      ],
    }
    const { model } = build([frame], {
      overrides: [{ match: { field: "secret" }, hidden: true }],
    })
    expect(model.bars.map((bar) => bar.name)).toEqual(["shown"])
  })

  it("keeps a distinct key for series that share a name", () => {
    const { model } = build([series([1]), series([2])])
    expect(new Set(model.bars.map((bar) => bar.key)).size).toBe(2)
  })

  it("has no bars without data", () => {
    expect(build([]).model.bars).toEqual([])
  })
})

describe("reducer", () => {
  const frames = [series([5, 9, 2, null])]

  it("reduces with the last non-null value by default", () => {
    expect(build(frames).model.bars[0].value).toBe(2)
  })

  it("reduces with the chosen reducer", () => {
    expect(build(frames, {}, { reduce: "max" }).model.bars[0].value).toBe(9)
    expect(build(frames, {}, { reduce: "sum" }).model.bars[0].value).toBe(16)
  })
})

describe("range", () => {
  const frames = [series([2], { pod: "a" }), series([8], { pod: "b" })]

  it("runs from zero to the largest value", () => {
    const [a, b] = build(frames).model.bars
    expect([a.min, a.max]).toEqual([0, 8])
    expect([a.fraction, b.fraction]).toEqual([0.25, 1])
  })

  it("starts below zero when a value is negative", () => {
    const { model } = build([series([-4]), series([4])])
    expect(model.bars[0].min).toBe(-4)
    expect(model.bars.map((bar) => bar.fraction)).toEqual([0, 1])
  })

  it("uses the configured min and max", () => {
    const [a, b] = build(frames, { min: 0, max: 16 }).model.bars
    expect([a.min, a.max]).toEqual([0, 16])
    expect([a.fraction, b.fraction]).toEqual([0.125, 0.5])
  })

  it("keeps the data range for a bound the field leaves out", () => {
    const [a] = build(frames, { max: 20 }).model.bars
    expect([a.min, a.max]).toEqual([0, 20])
  })

  it("spans 0 to 100 for percent and 0 to 1 for percentunit", () => {
    const percent = build([series([25])], { unit: "percent" }).model.bars[0]
    expect([percent.min, percent.max, percent.fraction]).toEqual([0, 100, 0.25])
    const unit = build([series([0.5])], { unit: "percentunit" }).model.bars[0]
    expect([unit.min, unit.max, unit.fraction]).toEqual([0, 1, 0.5])
  })

  it("lets a configured bound replace the one a percent unit implies", () => {
    const [bar] = build([series([25])], { unit: "percent", max: 50 }).model.bars
    expect([bar.min, bar.max, bar.fraction]).toEqual([0, 50, 0.5])
  })

  it("lets an override give one field its own range", () => {
    const { model } = build(frames, {
      overrides: [{ match: { series: '{pod="a"}' }, min: 0, max: 4 }],
    })
    const [a, b] = model.bars
    expect([a.max, a.fraction]).toEqual([4, 0.5])
    expect([b.max, b.fraction]).toEqual([8, 1])
  })

  it("clamps the fill to the range", () => {
    const [a, b] = build(frames, { min: 3, max: 5 }).model.bars
    expect(a.fraction).toBe(0)
    expect(b.fraction).toBe(1)
  })

  it("widens an empty range by one", () => {
    const { model } = build([series([0])])
    expect([model.bars[0].min, model.bars[0].max]).toEqual([0, 1])
    const flat = build([series([5])], { min: 5, max: 5 }).model.bars[0]
    expect([flat.min, flat.max, flat.fraction]).toEqual([5, 6, 0])
    const inverted = build([series([5])], { min: 9, max: 3 }).model.bars[0]
    expect([inverted.min, inverted.max]).toEqual([9, 10])
  })

  it("draws an empty bar for a value that is null", () => {
    const { model } = build([series([null])], { min: -5, max: 5 })
    const [bar] = model.bars
    expect(bar.value).toBeNull()
    expect(bar.fraction).toBe(0)
    expect(bar.text).toBe("-")
  })
})

describe("text and color", () => {
  const frames = [series([0.2]), series([0.9], { pod: "b" })]
  const thresholds = {
    thresholds: [
      { color: "ok" },
      { value: 0.5, color: "warning" },
    ] as DataPanel["thresholds"],
  }

  it("shares decimals between bars of one unit", () => {
    const { model } = build([series([2.4]), series([2.45])])
    expect(model.bars.map((bar) => bar.text)).toEqual(["2.40", "2.45"])
  })

  it("leaves a field that fixes its own decimals out of the shared count", () => {
    const { model } = build(
      [series([2.4], { pod: "a" }), series([7.123], { pod: "b" })],
      { overrides: [{ match: { series: '{pod="b"}' }, decimals: 0 }] }
    )
    expect(model.bars.map((bar) => bar.text)).toEqual(["2.4", "7"])
  })

  it("shares decimals per unit, not across units", () => {
    const { model } = build(
      [series([2.4], { pod: "a" }), series([2.45], { pod: "b" })],
      { overrides: [{ match: { series: '{pod="b"}' }, unit: "bytes" }] }
    )
    expect(model.bars.map((bar) => bar.text)).toEqual(["2.4", "2.45 B"])
  })

  it("colors a bar by its threshold with the level and word", () => {
    const { model } = build(frames, thresholds)
    const [low, high] = model.bars
    expect([low.threshold, low.word, low.color]).toEqual([
      "ok",
      "OK",
      "var(--threshold-ok)",
    ])
    expect([high.threshold, high.word, high.color]).toEqual([
      "warning",
      "Warning",
      "var(--threshold-warning)",
    ])
  })

  it("leaves a bar without thresholds uncolored at the base level", () => {
    const [bar] = build(frames).model.bars
    expect([bar.color, bar.threshold, bar.word]).toEqual([null, "base", ""])
  })

  it("colors a bar by its fixed color", () => {
    const [bar] = build(frames, { color: "info" }).model.bars
    expect([bar.color, bar.threshold]).toEqual([
      "var(--threshold-info)",
      "base",
    ])
  })

  it("lets a mapping color replace the threshold with no state word", () => {
    const { model } = build(frames, {
      ...thresholds,
      mappings: [{ value: "0.9", text: "Down", color: "critical" }],
    })
    const high = model.bars[1]
    expect([high.text, high.color, high.threshold, high.word]).toEqual([
      "Down",
      "var(--threshold-critical)",
      "base",
      "",
    ])
  })

  it("judges percentage thresholds against the bar's range", () => {
    const { model } = build([series([40]), series([80], { pod: "b" })], {
      thresholdMode: "percentage",
      thresholds: [
        { color: "ok" },
        { value: 60, color: "critical" },
      ] as DataPanel["thresholds"],
    })
    expect(model.bars.map((bar) => bar.threshold)).toEqual(["ok", "critical"])
  })
})

describe("data links", () => {
  const link = {
    title: "Pod ${__field.labels.pod}",
    url: "/pods/${__field.labels.pod}?v=${__value.raw}&n=${__series.name}&f=${__field.name}&t=${__value.text}",
  }
  const plain = { title: "Docs", url: "https://example.com/docs" }

  function links(
    extra: Partial<DataPanel>,
    frames = [series([3], { pod: "a" })]
  ) {
    const { fields, display, model } = build(frames, extra)
    return barLinks(fields, model.bars[0], reducerOf(display), {
      vars: {},
      fields,
    })
  }

  it("passes the frame's labels, the value and the names", () => {
    expect(links({ links: [link] })).toEqual([
      {
        title: "Pod a",
        href: "/pods/a?v=3&n=%7Bpod%3D%22a%22%7D&f=Value&t=3",
        newTab: false,
      },
    ])
  })

  it("leaves out links that read no data", () => {
    expect(links({ links: [plain] })).toEqual([])
  })

  it("drops a link to an unsafe address", () => {
    const unsafe = { title: "Bad", url: "javascript:alert(${__value.raw})" }
    expect(links({ links: [unsafe, link] })).toHaveLength(1)
  })

  it("resolves a row token from the reduced values of the frame", () => {
    const frame: PreparedFrame = {
      ref: "A",
      fields: [
        { name: "Time", type: "time", values: [1, 2, 3] },
        { name: "host", type: "string", values: ["old", "new", null] },
        { name: "Value", type: "number", values: [4, 7, null] },
        { name: "other", type: "number", values: [1, 9, null] },
      ],
    }
    const row = {
      title: "Open",
      url: "/h/${__data.fields.host}?o=${__data.fields.other}&v=${__data.fields.Value}",
    }
    const { fields, display, model } = build([frame], { links: [row] })
    const [first] = model.bars
    expect(
      barLinks(fields, first, reducerOf(display), { vars: {}, fields })
    ).toEqual([{ title: "Open", href: "/h/new?o=9&v=7", newTab: false }])
  })
})

describe("bargaugeModel value options", () => {
  it("draws one bar per row with allValues", () => {
    const { model } = build([series([10, 40, 90])], {}, { allValues: true })
    expect(model.bars.map((bar) => bar.value)).toEqual([10, 40, 90])
  })

  it("stops at the row limit", () => {
    const { model } = build(
      [series([10, 40, 90])],
      {},
      { allValues: true, limit: 1 }
    )
    expect(model.bars.map((bar) => bar.value)).toEqual([10])
  })

  it("links a row bar to its own row", () => {
    const link = {
      title: "Row",
      url: "/r?v=${__value.raw}&t=${__data.fields.Time}",
    }
    const { fields, display, model } = build(
      [series([10, 40, 90])],
      { links: [link] } as Partial<DataPanel>,
      { allValues: true }
    )
    const resolved = barLinks(fields, model.bars[1], reducerOf(display), {
      vars: {},
      fields,
    })
    expect(resolved[0].href).toBe("/r?v=40&t=1")
  })

  it("shows only the fields a pattern picks", () => {
    const frame = series(
      [5],
      {},
      {
        fields: [
          { name: "Time", type: "time", values: [0] },
          { name: "cpu", type: "number", values: [1] },
          { name: "mem", type: "number", values: [2] },
        ],
      }
    )
    const { model } = build([frame], {}, { fields: "/^m/" })
    expect(model.bars.map((bar) => bar.value)).toEqual([2])
  })
})

describe("name placement", () => {
  it.each([
    [{}, "horizontal", "above"],
    [{ namePlacement: "auto" }, "horizontal", "above"],
    [{ namePlacement: "top" }, "horizontal", "above"],
    [{ namePlacement: "left" }, "horizontal", "left"],
    [{ namePlacement: "hidden" }, "horizontal", "hidden"],
    [{}, "vertical", "below"],
    [{ namePlacement: "auto" }, "vertical", "below"],
    [{ namePlacement: "top" }, "vertical", "above"],
    [{ namePlacement: "left" }, "vertical", "below"],
    [{ namePlacement: "hidden" }, "vertical", "hidden"],
  ] as const)(
    "places the name for %j in a %s bar gauge: %s",
    (display, orientation, expected) => {
      expect(namePlacementOf(display, orientation)).toBe(expected)
    }
  )
})

describe("bar gauge style options", () => {
  it("reads the bar style, value mode and unfilled area with the draw defaults", () => {
    expect(barStyleOf({})).toBe("basic")
    expect(barStyleOf({ barStyle: "lcd" })).toBe("lcd")
    expect(valueModeOf({})).toBe("color")
    expect(valueModeOf({ valueMode: "text" })).toBe("text")
    expect(valueModeOf({ valueMode: "hidden" })).toBe("hidden")
    expect(showUnfilledOf({})).toBe(true)
    expect(showUnfilledOf({ showUnfilled: false })).toBe(false)
  })

  it("reads size limits and text sizes only when they are positive numbers", () => {
    expect(barLimits({ minWidth: 8, minHeight: 16, maxHeight: 300 })).toEqual({
      minWidth: 8,
      minHeight: 16,
      maxHeight: 300,
    })
    expect(barLimits({ minHeight: 0, maxHeight: "30" })).toEqual({
      minWidth: undefined,
      minHeight: undefined,
      maxHeight: undefined,
    })
    expect(barSizes({ titleSize: 12, valueSize: 30 })).toEqual({
      titleSize: 12,
      valueSize: 30,
    })
  })
})

describe("lcd cells", () => {
  it.each([
    [0, 20, 0],
    [-1, 20, 0],
    [Number.NaN, 20, 0],
    [0.01, 20, 1],
    [0.5, 20, 10],
    [0.74, 20, 15],
    [1, 20, 20],
    [2, 20, 20],
  ])("lights %d of %d cells as %d", (fraction, cells, lit) => {
    expect(lcdLitCells(fraction, cells)).toBe(lit)
  })

  it.each([
    [0, 4],
    [35, 4],
    [100, 10],
    [259, 25],
    [10000, 40],
  ])("draws a bar %dpx long with %d cells", (extent, cells) => {
    expect(lcdCellCount(extent)).toBe(cells)
  })
})
