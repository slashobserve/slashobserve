import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  axisTicks,
  barRect,
  barchartModel,
  labelSpot,
  labelStride,
  maxCategories,
  noCategoriesReason,
  orientationOf,
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
  size = wide
) {
  return barchartModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("barchart", display),
    size
  )
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
