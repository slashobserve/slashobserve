import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  emptyTail,
  keepRows,
  mergeTail,
  newestTime,
  rowCount,
  tailWindow,
} from "./frames"

function logFrame(
  times: number[],
  labels: Record<string, string> = { service_name: "checkout" }
): WireFrame {
  return {
    ref: "A",
    labels,
    fields: [
      { name: "Time", type: "time", values: times },
      { name: "Body", type: "string", values: times.map((t) => `line ${t}`) },
    ],
  }
}

describe("keepRows", () => {
  it("slices every field by the same rows and keeps the labels", () => {
    const frame = logFrame([1, 2, 3, 4])
    const kept = keepRows([frame], (_, row) => row % 2 === 1)
    expect(kept).toEqual([
      {
        ref: "A",
        labels: { service_name: "checkout" },
        fields: [
          { name: "Time", type: "time", values: [2, 4] },
          { name: "Body", type: "string", values: ["line 2", "line 4"] },
        ],
      },
    ])
    expect(frame.fields[0].values).toEqual([1, 2, 3, 4])
  })

  it("drops frames left empty", () => {
    expect(keepRows([logFrame([1]), logFrame([2])], () => false)).toEqual([])
  })
})

describe("mergeTail", () => {
  it("drops a row already seen at the window boundary", () => {
    const seeded = mergeTail(emptyTail, [logFrame([100, 200])])
    const next = mergeTail(seeded, [logFrame([200, 300])])
    expect(rowCount(next.frames)).toBe(3)
    expect(next.newest).toBe(300)
  })

  it("keeps two rows that differ only by their labels", () => {
    const merged = mergeTail(emptyTail, [
      logFrame([100], { pod: "a" }),
      logFrame([100], { pod: "b" }),
    ])
    expect(rowCount(merged.frames)).toBe(2)
  })

  it("keeps the newest rows up to the cap", () => {
    const times = Array.from({ length: 1200 }, (_, i) => 1_000 + i)
    const merged = mergeTail(emptyTail, [logFrame(times)])
    expect(rowCount(merged.frames)).toBe(1000)
    const kept = merged.frames[0].fields[0].values as number[]
    expect(Math.min(...kept)).toBe(1_200)
    expect(Math.max(...kept)).toBe(2_199)
  })

  it("advances the newest time and leaves it when nothing arrives", () => {
    const first = mergeTail(emptyTail, [logFrame([5, 9])])
    expect(first.newest).toBe(9)
    expect(mergeTail(first, []).newest).toBe(9)
    expect(newestTime(mergeTail(first, [logFrame([12])]).frames)).toBe(12)
  })
})

describe("tailWindow", () => {
  it.each([
    ["no line yet", null, 50_000, { from: "40000", to: "51000" }],
    ["a line in the past", 45_000, 50_000, { from: "45000", to: "50000" }],
    [
      "a line ahead of the browser clock",
      52_000,
      50_000,
      { from: "52000", to: "53000" },
    ],
  ])("covers %s", (_, newest, now, window) => {
    expect(tailWindow(newest, now)).toEqual(window)
  })
})
