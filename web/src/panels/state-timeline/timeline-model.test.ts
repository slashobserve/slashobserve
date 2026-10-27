import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { PreparedFrame } from "@/panels/transforms"
import { displayWithDefaults, type DataPanel } from "@/spec"

import { mergeValuesOf, segmentAt, timelineModel } from "./timeline-model"

const range = { from: 0, to: 100 }

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "States",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "state-timeline",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

function series(
  name: string,
  values: unknown[],
  type: "number" | "string" | "bool" = "number",
  times = values.map((_, index) => index * 10)
): PreparedFrame {
  return {
    ref: "A",
    name,
    labels: {},
    fields: [
      { name: "Time", type: "time", values: times },
      { name: type === "number" ? "Value" : name, type, values },
    ],
  } as PreparedFrame
}

function build(
  frames: PreparedFrame[],
  extra: Partial<DataPanel> = {},
  display: Record<string, unknown> = {},
  window = range,
  cells = false
) {
  return timelineModel(
    frames,
    createFieldResolver(panel(extra)),
    displayWithDefaults("state-timeline", display),
    window,
    cells
  )
}

describe("timelineModel", () => {
  it("merges equal neighbours by default", () => {
    const { rows } = build([series("a", [1, 1, 2, 2, 1])])
    expect(
      rows[0].segments.map(({ from, to, text }) => [from, to, text])
    ).toEqual([
      [0, 20, "1"],
      [20, 40, "2"],
      [40, 100, "1"],
    ])
  })

  it("keeps every point when merging is off", () => {
    const { rows } = build(
      [series("a", [1, 1, 2, 2, 1])],
      {},
      {
        mergeValues: false,
      }
    )
    expect(rows[0].segments).toHaveLength(5)
    expect(rows[0].segments[1]).toMatchObject({ from: 10, to: 20, text: "1" })
  })

  it("leaves a gap for a null value and does not merge across it", () => {
    const { rows } = build([series("a", [1, null, 1])])
    expect(rows[0].segments.map(({ from, to }) => [from, to])).toEqual([
      [0, 10],
      [20, 100],
    ])
  })

  it("lets the last value last until the end of the range", () => {
    const { rows } = build([series("a", [1, 2])], {}, {}, { from: 0, to: 55 })
    expect(rows[0].segments.at(-1)).toMatchObject({ from: 10, to: 55 })
  })

  it("clips samples outside the range", () => {
    const { rows } = build(
      [series("a", [1, 2, 3], "number", [-20, 40, 200])],
      {},
      {}
    )
    expect(
      rows[0].segments.map(({ from, to, text }) => [from, to, text])
    ).toEqual([
      [0, 40, "1"],
      [40, 100, "2"],
    ])
  })

  it("orders samples by time before cutting segments", () => {
    const { rows } = build([series("a", [2, 1], "number", [50, 0])])
    expect(
      rows[0].segments.map(({ from, to, text }) => [from, to, text])
    ).toEqual([
      [0, 50, "1"],
      [50, 100, "2"],
    ])
  })

  it("draws one row per visible field that is not time", () => {
    const frames = [
      series("a", [1]),
      series("b", ["x"], "string"),
      series("c", [true], "bool"),
    ]
    const { rows } = build(frames)
    expect(rows.map((row) => row.name)).toEqual(["a", "b", "c"])
  })

  it("skips hidden fields and frames without a time field", () => {
    const noTime: PreparedFrame = {
      ref: "B",
      name: "b",
      labels: {},
      fields: [{ name: "Value", type: "number", values: [1] }],
    } as PreparedFrame
    const { rows } = build([series("a", [1]), noTime], {
      overrides: [{ match: { series: "a" }, hidden: true }],
    } as Partial<DataPanel>)
    expect(rows).toEqual([])
  })

  it("names rows by display name", () => {
    const { rows } = build([series("a", [1])], { displayName: "Pods" })
    expect(rows[0].name).toBe("Pods")
  })

  it("uses mapping text and colors", () => {
    const { rows, states } = build([series("a", [1, 0])], {
      mappings: [
        { value: "1", text: "Serving", color: "ok" },
        { value: "0", text: "Idle", color: "warning" },
      ],
    } as Partial<DataPanel>)
    expect(rows[0].segments.map(({ text, color }) => [text, color])).toEqual([
      ["Serving", "var(--threshold-ok)"],
      ["Idle", "var(--threshold-warning)"],
    ])
    expect(states).toEqual([
      { text: "Serving", color: "var(--threshold-ok)" },
      { text: "Idle", color: "var(--threshold-warning)" },
    ])
  })

  it("colors by threshold when there is no mapping", () => {
    const { rows } = build([series("a", [1, 90])], {
      thresholds: [{ color: "ok" }, { value: 50, color: "critical" }],
    } as Partial<DataPanel>)
    expect(rows[0].segments.map(({ color }) => color)).toEqual([
      "var(--threshold-ok)",
      "var(--threshold-critical)",
    ])
  })

  it("gives each distinct text one palette color in order of first appearance", () => {
    const { rows, states } = build([
      series("a", ["up", "down", "up"], "string"),
      series("b", ["warm", "up"], "string"),
    ])
    expect(states.map(({ text }) => text)).toEqual(["up", "down", "warm"])
    expect(states.map(({ color }) => color)).toEqual([
      "var(--series-1)",
      "var(--series-2)",
      "var(--series-3)",
    ])
    expect(rows[1].segments[1].color).toBe("var(--series-1)")
  })

  it("lists no states for an empty panel", () => {
    expect(build([])).toEqual({ rows: [], states: [] })
  })

  it("distinguishes numbers by display text", () => {
    const { states } = build([series("a", [0.5, 1.5])], { unit: "percentunit" })
    expect(states.map(({ text }) => text)).toEqual(["50%", "150%"])
  })
})

describe("timelineModel with cells", () => {
  const cells = (frames: PreparedFrame[], extra: Partial<DataPanel> = {}) =>
    build(frames, extra, {}, range, true)

  it("keeps equal neighbours as separate cells", () => {
    const { rows } = cells([series("a", [1, 1, 2, 2, 1])])
    expect(
      rows[0].segments.map(({ from, to, text }) => [from, to, text])
    ).toEqual([
      [0, 10, "1"],
      [10, 20, "1"],
      [20, 30, "2"],
      [30, 40, "2"],
      [40, 100, "1"],
    ])
  })

  it("ignores mergeValues", () => {
    const { rows } = build(
      [series("a", [1, 1])],
      {},
      { mergeValues: true },
      range,
      true
    )
    expect(rows[0].segments).toHaveLength(2)
  })

  it("colors states and leaves a gap for null the same way", () => {
    const frames = [series("a", ["up", null, "up", "down"], "string")]
    const merged = build(frames)
    const split = cells(frames)
    expect(split.states).toEqual(merged.states)
    expect(
      split.rows[0].segments.map(({ from, to, color }) => [from, to, color])
    ).toEqual([
      [0, 10, "var(--series-1)"],
      [20, 30, "var(--series-1)"],
      [30, 100, "var(--series-2)"],
    ])
  })
})

describe("mergeValuesOf", () => {
  it("is on unless written false", () => {
    expect(mergeValuesOf({})).toBe(true)
    expect(mergeValuesOf({ mergeValues: false })).toBe(false)
  })
})

describe("segmentAt", () => {
  const segments = [
    { from: 0, to: 10, text: "a", color: "red" },
    { from: 20, to: 30, text: "b", color: "blue" },
  ]

  it("finds the segment that holds the time", () => {
    expect(segmentAt(segments, 0)?.text).toBe("a")
    expect(segmentAt(segments, 25)?.text).toBe("b")
  })

  it("finds nothing in a gap, at an end or outside", () => {
    expect(segmentAt(segments, 10)).toBeNull()
    expect(segmentAt(segments, 15)).toBeNull()
    expect(segmentAt(segments, 30)).toBeNull()
    expect(segmentAt(segments, -1)).toBeNull()
    expect(segmentAt([], 5)).toBeNull()
  })
})
