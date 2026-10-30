import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  exemplarAt,
  exemplarPoints,
  type ExemplarPoint,
  type ExemplarSeries,
} from "./exemplars"

function exemplarFrame(
  ref: string,
  labels: Record<string, string>,
  rows: { time: number; value: number | null; text?: Record<string, string> }[]
): WireFrame {
  const keys = [...new Set(rows.flatMap((row) => Object.keys(row.text ?? {})))]
  return {
    ref,
    labels,
    fields: [
      { name: "Time", type: "time", values: rows.map((row) => row.time) },
      { name: "Value", type: "number", values: rows.map((row) => row.value) },
      ...keys.map((key) => ({
        name: key,
        type: "string" as const,
        values: rows.map((row) => row.text?.[key] ?? ""),
      })),
    ],
  }
}

const series: ExemplarSeries[] = [
  { key: "a-web", ref: "A", labels: { pod: "web" }, scale: "y" },
  { key: "a-api", ref: "A", labels: { pod: "api" }, scale: "y2" },
  { key: "b-web", ref: "B", labels: { pod: "web" }, scale: "y" },
]

describe("exemplarPoints", () => {
  it("matches a frame to the series with the same ref and labels", () => {
    const points = exemplarPoints(
      [
        exemplarFrame("A", { pod: "api" }, [{ time: 10_000, value: 3 }]),
        exemplarFrame("B", { pod: "web" }, [{ time: 20_000, value: 5 }]),
      ],
      series,
      [0, 100]
    )
    expect(points).toEqual([
      { x: 10, y: 3, seriesKey: "a-api", scale: "y2", labels: [] },
      { x: 20, y: 5, seriesKey: "b-web", scale: "y", labels: [] },
    ])
  })

  it("puts an exemplar with no matching series on the default axis", () => {
    const points = exemplarPoints(
      [
        exemplarFrame("A", { pod: "db" }, [{ time: 10_000, value: 3 }]),
        exemplarFrame("C", { pod: "web" }, [{ time: 11_000, value: 4 }]),
        exemplarFrame("A", { pod: "web", zone: "x" }, [
          { time: 12_000, value: 6 },
        ]),
      ],
      series,
      [0, 100]
    )
    expect(points.map(({ seriesKey, scale }) => [seriesKey, scale])).toEqual([
      [undefined, "y"],
      [undefined, "y"],
      [undefined, "y"],
    ])
  })

  it("keeps only exemplars inside the visible range, in seconds", () => {
    const points = exemplarPoints(
      [
        exemplarFrame("A", { pod: "web" }, [
          { time: 4_999, value: 1 },
          { time: 5_000, value: 2 },
          { time: 9_000, value: 3 },
          { time: 10_000, value: 4 },
          { time: 10_001, value: 5 },
        ]),
      ],
      series,
      [5, 10]
    )
    expect(points.map((point) => point.x)).toEqual([5, 9, 10])
  })

  it("skips rows without a finite value", () => {
    const points = exemplarPoints(
      [
        exemplarFrame("A", { pod: "web" }, [
          { time: 1_000, value: null },
          { time: 2_000, value: 2 },
        ]),
      ],
      series,
      [0, 100]
    )
    expect(points.map((point) => point.y)).toEqual([2])
  })

  it("lists the trace id label first and drops empty labels", () => {
    const points = exemplarPoints(
      [
        exemplarFrame("A", { pod: "web" }, [
          { time: 1_000, value: 1, text: { span: "s1", traceID: "abc" } },
          { time: 2_000, value: 2, text: { span: "", traceID: "def" } },
          { time: 3_000, value: 3, text: { span: "s3", traceID: "" } },
        ]),
      ],
      series,
      [0, 100]
    )
    expect(points.map((point) => point.labels)).toEqual([
      [
        ["traceID", "abc"],
        ["span", "s1"],
      ],
      [["traceID", "def"]],
      [["span", "s3"]],
    ])
  })

  it("returns nothing for no frames or a frame without time and value", () => {
    expect(exemplarPoints([], series, [0, 100])).toEqual([])
    expect(
      exemplarPoints(
        [{ ref: "A", fields: [{ name: "x", type: "string", values: ["a"] }] }],
        series,
        [0, 100]
      )
    ).toEqual([])
  })
})

describe("exemplarAt", () => {
  const point = (x: number): ExemplarPoint => ({
    x,
    y: 0,
    scale: "y",
    labels: [],
  })
  const toPx = (p: ExemplarPoint): [number, number] => [p.x, 50]

  it("returns the nearest point within the radius", () => {
    const points = [point(100), point(104), point(200)]
    expect(exemplarAt(points, 103, 50, 6, toPx)).toBe(points[1])
    expect(exemplarAt(points, 101, 52, 6, toPx)).toBe(points[0])
  })

  it("counts the vertical distance", () => {
    expect(exemplarAt([point(100)], 100, 57, 6, toPx)).toBeNull()
    expect(exemplarAt([point(100)], 100, 56, 6, toPx)).not.toBeNull()
  })

  it("returns null when nothing is within the radius", () => {
    expect(exemplarAt([point(100)], 150, 50, 6, toPx)).toBeNull()
    expect(exemplarAt([], 0, 0, 6, toPx)).toBeNull()
  })
})
