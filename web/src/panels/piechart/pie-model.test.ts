import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import {
  legendOf,
  percentText,
  pieModel,
  pieTypeOf,
  slicePath,
  summaryLabel,
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

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {}
) {
  return pieModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("piechart", display)
  )
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

  it("leaves null, zero and negative values out of the pie but lists them", () => {
    const { slices, left, total } = build([
      series("a", [4]),
      series("b", [null]),
      series("c", [0]),
      series("d", [-2]),
    ])
    expect(total).toBe(4)
    expect(slices.map((slice) => slice.name)).toEqual(["a"])
    expect(left.map((item) => item.name)).toEqual(["b", "c", "d"])
    expect(left.map((item) => item.text)).toEqual(["-", "0", "-2"])
    expect(left.map((item) => item.value)).toEqual([null, 0, -2])
  })

  it("draws nothing when no value is positive", () => {
    const { slices, left, total } = build([
      series("a", [0]),
      series("b", [null]),
    ])
    expect(total).toBe(0)
    expect(slices).toEqual([])
    expect(left).toHaveLength(2)
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
    const { slices, left } = build(
      [series("a", [1]), series("b", [null]), series("c", [2])],
      { color: undefined }
    )
    expect(slices.map((slice) => slice.color)).toEqual([
      "var(--series-1)",
      "var(--series-3)",
    ])
    expect(left[0].color).toBe("var(--series-2)")
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

describe("display options", () => {
  it("reads pie type and legend placement", () => {
    expect(pieTypeOf({ pieType: "donut" })).toBe("donut")
    expect(pieTypeOf({})).toBe("pie")
    expect(legendOf({ legend: "bottom" })).toBe("bottom")
    expect(legendOf({ legend: "hidden" })).toBe("hidden")
    expect(legendOf({})).toBe("right")
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
