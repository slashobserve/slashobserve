import { describe, expect, it } from "vitest"

import { hitTest, locate, type BarAnchorLayout } from "./anchors"
import type { Bar, Layout } from "./barchart-model"

function bar(value: number | null, start: number, end: number): Bar {
  return {
    value,
    shown: {
      text: value === null ? "" : `${value} ms`,
      numeric: value,
      color: null,
      threshold: { level: "base", color: "", label: "" },
      mapped: false,
    },
    color: "var(--series-1)",
    start,
    end,
    roundEnd: false,
  } as Bar
}

const layout: Layout = {
  orientation: "vertical",
  plot: { width: 200, height: 100 },
  min: 0,
  max: 10,
  scale: { kind: "linear" },
  count: 2,
  slotCount: 2,
  groupWidth: 1,
  barWidth: 1,
} as Layout

function surface(stack: "none" | "normal"): BarAnchorLayout {
  return {
    body: { width: 240, height: 140 },
    origin: { x: 20, y: 10 },
    layout,
    model: {
      categories: [
        { key: "a", label: "checkout", title: "checkout" },
        { key: "b", label: "payments", title: "payments" },
      ],
      series: [
        { name: "p50", stack, slot: 0 },
        { name: "p99", stack, slot: 1 },
      ],
      bars: [
        [bar(5, 0, 5), bar(10, 0, 10)],
        [bar(2, 0, 2), bar(null, 0, 0)],
      ],
    } as unknown as BarAnchorLayout["model"],
  }
}

describe("barchart hitTest", () => {
  it("names the bar under the pointer", () => {
    expect(hitTest(surface("none"), { x: 20 + 20, y: 10 + 80 })).toEqual({
      kind: "bar",
      pos: { x: 40 / 240, y: 90 / 140 },
      category: "checkout",
      series: { name: "p50" },
      shown: "5 ms",
    })
  })

  it("names a segment when the series stack", () => {
    expect(
      hitTest(surface("normal"), { x: 20 + 80, y: 10 + 50 })
    ).toMatchObject({
      kind: "segment",
      category: "checkout",
      series: { name: "p99" },
    })
  })

  it("names the category when the pointer is above the bars", () => {
    expect(hitTest(surface("none"), { x: 20 + 150, y: 10 + 5 })).toEqual({
      kind: "bar",
      pos: { x: 170 / 240, y: 15 / 140 },
      category: "payments",
    })
  })

  it("returns null outside the plot", () => {
    expect(hitTest(surface("none"), { x: 5, y: 40 })).toBeNull()
    expect(hitTest(surface("none"), { x: 100, y: 130 })).toBeNull()
  })
})

describe("barchart locate", () => {
  const pos = { x: 0, y: 0 }

  it("finds the bar a series names inside its category", () => {
    const rect = locate(surface("none"), {
      kind: "bar",
      pos,
      category: "checkout",
      series: { name: "p50" },
    })
    expect(rect).not.toBeNull()
    expect(rect!.x).toBeGreaterThanOrEqual(20)
    expect(rect!.x + rect!.width).toBeLessThanOrEqual(20 + 100)
    expect(
      hitTest(surface("none"), {
        x: rect!.x + rect!.width / 2,
        y: rect!.y + rect!.height - 1,
      })
    ).toMatchObject({ category: "checkout", series: { name: "p50" } })
  })

  it("finds the whole category when no series is named", () => {
    const rect = locate(surface("none"), {
      kind: "bar",
      pos,
      category: "payments",
    })
    expect(rect).toMatchObject({ x: 120, y: 10 })
  })

  it("returns null when the category or the series is gone", () => {
    expect(
      locate(surface("none"), { kind: "bar", pos, category: "search" })
    ).toBeNull()
    expect(
      locate(surface("none"), {
        kind: "bar",
        pos,
        category: "checkout",
        series: { name: "p75" },
      })
    ).toBeNull()
  })
})
