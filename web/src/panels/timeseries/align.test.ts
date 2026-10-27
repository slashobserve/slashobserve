import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import { prepareFrames, seriesName } from "../transforms"
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

function align(frames: WireFrame[], legends: Record<string, string> = {}) {
  const panel = {
    queries: ["A", "B"].map((ref) => ({ query: ref, legend: legends[ref] })),
  } as DataPanel
  return alignFrames(prepareFrames(panel, frames), seriesName)
}

describe("alignFrames", () => {
  it("labels each frame with the legend of its own query", () => {
    const aligned = align(
      [frame("A", { pod: "x" }, [1, 2]), frame("B", { pod: "x" }, [3, 4])],
      { A: "cpu {{pod}}", B: "mem {{pod}}" }
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

  it("names a series by its field and label value when a query has no legend", () => {
    const aligned = align([frame("A", { pod: "x" }, [1, 2])])
    expect(aligned.series[0].label).toBe("value x")
  })

  it("names series of two queries by their ref", () => {
    const aligned = align([frame("A", {}, [1, 2]), frame("B", {}, [3, 4])])
    expect(aligned.series.map((s) => s.label)).toEqual(["A value", "B value"])
  })

  it("does not prefix a shared templated legend that resolves differently", () => {
    const aligned = align(
      [frame("A", { pod: "x" }, [1, 2]), frame("B", { pod: "y" }, [3, 4])],
      { A: "{{pod}}", B: "{{pod}}" }
    )
    expect(aligned.series.map((s) => s.label)).toEqual(["x", "y"])
  })

  it("does not prefix labels that collide inside one query", () => {
    const aligned = align([frame("A", {}, [1, 2]), frame("A", {}, [3, 4])])
    expect(aligned.series.map((s) => s.label)).toEqual(["value", "value"])
  })

  it("merges ghost timestamps into the shared x values without labelling them", () => {
    const ghost = prepareFrames({ queries: [] } as unknown as DataPanel, [
      {
        ref: "A",
        labels: {},
        fields: [
          { name: "time", type: "time", values: [1500, 2000] },
          { name: "value", type: "number", values: [7, 8] },
        ],
      } as WireFrame,
    ])
    const aligned = alignFrames(
      prepareFrames({ queries: [] } as unknown as DataPanel, [
        frame("A", { pod: "x" }, [1, 2]),
      ]),
      seriesName,
      ghost
    )
    expect(aligned.xs).toEqual([1, 1.5, 2])
    expect(aligned.series.map((s) => s.values)).toEqual([[1, undefined, 2]])
    expect(aligned.ghosts).toEqual([[undefined, 7, 8]])
  })

  it("keeps a real null as a gap and an unmatched slot as undefined", () => {
    const withGap = {
      ref: "A",
      labels: { pod: "x" },
      fields: [
        { name: "time", type: "time", values: [1000, 2000, 3000] },
        { name: "value", type: "number", values: [1, null, 3] },
      ],
    } as WireFrame
    const sparse = {
      ref: "B",
      labels: { pod: "y" },
      fields: [
        { name: "time", type: "time", values: [1000, 3000] },
        { name: "value", type: "number", values: [4, 6] },
      ],
    } as WireFrame
    const aligned = align([withGap, sparse])
    expect(aligned.series.map((s) => s.values)).toEqual([
      [1, null, 3],
      [4, undefined, 6],
    ])
  })

  it("leaves a hole where one query has a different step", () => {
    const fast = {
      ref: "A",
      labels: { pod: "x" },
      fields: [
        { name: "time", type: "time", values: [1000, 2000, 3000, 4000] },
        { name: "value", type: "number", values: [1, 2, 3, 4] },
      ],
    } as WireFrame
    const slow = {
      ref: "B",
      labels: { pod: "y" },
      fields: [
        { name: "time", type: "time", values: [1000, 3000] },
        { name: "value", type: "number", values: [10, 30] },
      ],
    } as WireFrame
    const aligned = align([fast, slow])
    expect(aligned.xs).toEqual([1, 2, 3, 4])
    expect(aligned.series.map((s) => s.values)).toEqual([
      [1, 2, 3, 4],
      [10, undefined, 30, undefined],
    ])
  })
})
