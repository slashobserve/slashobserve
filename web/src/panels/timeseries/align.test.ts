import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import { alignFrames } from "./align"

function frame(ref: string, labels: Record<string, string>, values: number[]) {
  return {
    ref,
    labels,
    fields: [
      { name: "time", type: "time", values: [1000, 2000] },
      { name: "value", type: "number", values },
    ],
  } as WireFrame
}

describe("alignFrames", () => {
  it("labels each frame with the legend of its own query", () => {
    const legends: Record<string, string | undefined> = {
      A: "cpu {{pod}}",
      B: "mem {{pod}}",
    }
    const aligned = alignFrames(
      [frame("A", { pod: "x" }, [1, 2]), frame("B", { pod: "x" }, [3, 4])],
      (f) => legends[f.ref]
    )
    expect(aligned.series.map((s) => [s.key, s.label])).toEqual([
      ["A:{pod=x}#value", "cpu x"],
      ["B:{pod=x}#value", "mem x"],
    ])
    expect(aligned.xs).toEqual([1, 2])
    expect(aligned.series.map((s) => s.values)).toEqual([
      [1, 2],
      [3, 4],
    ])
  })

  it("falls back to the label set when a query has no legend", () => {
    const aligned = alignFrames(
      [frame("A", { pod: "x" }, [1, 2])],
      () => undefined
    )
    expect(aligned.series[0].label).toBe('{pod="x"}')
  })

  it("prefixes a fallback label with the query ref when asked", () => {
    const aligned = alignFrames(
      [frame("A", {}, [1, 2]), frame("B", {}, [3, 4])],
      () => undefined,
      () => "fallback"
    )
    expect(aligned.series.map((s) => s.label)).toEqual(["A: value", "B: value"])
  })
})
