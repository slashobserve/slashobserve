import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"
import type { DataPanel } from "@/spec"

import {
  firstNumberSeries,
  hasData,
  lastNumber,
  legendFor,
  seriesKey,
  seriesLabel,
  uniqueKey,
} from "./frames"

const time: WireFrame["fields"][number] = {
  name: "time",
  type: "time",
  values: [1, 2, 3],
}

const noLegend = () => undefined

function frame(partial: Partial<WireFrame> & Pick<WireFrame, "fields">) {
  return { ref: "A", ...partial } as WireFrame
}

describe("seriesLabel", () => {
  const f = frame({
    name: "http_requests_total",
    labels: { pod: "checkout-0", status: "200" },
    fields: [time, { name: "value", type: "number", values: [1, 2, 3] }],
  })

  it("expands legend templates", () => {
    expect(seriesLabel(f, f.fields[1], "{{pod}} ({{ status }})", 1)).toBe(
      "checkout-0 (200)"
    )
    expect(seriesLabel(f, f.fields[1], "{{__name__}}-{{missing}}", 1)).toBe(
      "http_requests_total-"
    )
  })

  it("builds a Prometheus style label without a legend", () => {
    expect(seriesLabel(f, f.fields[1], undefined, 1)).toBe(
      'http_requests_total{pod="checkout-0", status="200"}'
    )
  })

  it("falls back to the frame name, then the field name", () => {
    const named = frame({ name: "up", fields: [time] })
    expect(seriesLabel(named, named.fields[0], undefined, 1)).toBe("up")
    const bare = frame({ fields: [time] })
    expect(seriesLabel(bare, bare.fields[0], undefined, 1)).toBe("time")
  })

  it("names an unlabelled, unnamed frame's number fields by their own name", () => {
    const wide = frame({
      fields: [
        { name: "a", type: "number", values: [1] },
        { name: "b", type: "number", values: [2] },
      ],
    })
    expect(seriesLabel(wide, wide.fields[0], undefined, 2)).toBe("a")
    expect(seriesLabel(wide, wide.fields[1], undefined, 2)).toBe("b")
  })

  it("appends the field name when a frame has several number fields", () => {
    expect(seriesLabel(f, f.fields[1], "{{pod}}", 2)).toBe("checkout-0 value")
  })
})

describe("lastNumber", () => {
  it("skips nulls and non-finite values", () => {
    expect(
      lastNumber({ name: "v", type: "number", values: [1, 2, null, NaN] })
    ).toBe(2)
    expect(lastNumber({ name: "v", type: "number", values: [null] })).toBeNull()
  })
})

describe("hasData", () => {
  it("is false without frames or without non-null values", () => {
    expect(hasData([])).toBe(false)
    expect(
      hasData([
        frame({
          fields: [time, { name: "v", type: "number", values: [null, null] }],
        }),
      ])
    ).toBe(false)
  })

  it("is true with a non-null value", () => {
    expect(
      hasData([
        frame({
          fields: [time, { name: "v", type: "number", values: [null, 1] }],
        }),
      ])
    ).toBe(true)
  })
})

describe("firstNumberSeries", () => {
  const value = (values: number[]) =>
    ({ name: "value", type: "number", values }) as const

  it("keys each series by frame identity, not by position", () => {
    const a = frame({ labels: { pod: "a" }, fields: [time, value([1])] })
    const b = frame({ labels: { pod: "b" }, fields: [time, value([2])] })
    const keys = (frames: WireFrame[]) =>
      firstNumberSeries(frames, () => "{{pod}}").map((s) => [s.label, s.key])
    expect(keys([a, b])).toEqual([
      ["a", "A:{pod=a}#value"],
      ["b", "A:{pod=b}#value"],
    ])
    expect(keys([b, a])).toEqual([
      ["b", "A:{pod=b}#value"],
      ["a", "A:{pod=a}#value"],
    ])
  })

  it("numbers keys only for identical frames", () => {
    const same = frame({ labels: { pod: "a" }, fields: [time, value([1])] })
    expect(
      firstNumberSeries([same, same, same], noLegend).map(
        (series) => series.key
      )
    ).toEqual(["A:{pod=a}#value", "A:{pod=a}#value#1", "A:{pod=a}#value#2"])
  })

  it("reads the last number of the first number field and skips frames without one", () => {
    const text = frame({
      fields: [time, { name: "msg", type: "string", values: ["x"] }],
    })
    const f = frame({ fields: [time, value([1, 2, 3])] })
    expect(
      firstNumberSeries([text, f], noLegend).map((series) => series.value)
    ).toEqual([3])
  })

  it("keeps the same series of two queries apart and labels each with its own legend", () => {
    const a = frame({
      ref: "A",
      labels: { pod: "x" },
      fields: [time, value([1])],
    })
    const b = frame({
      ref: "B",
      labels: { pod: "x" },
      fields: [time, value([2])],
    })
    const legends: Record<string, string> = {
      A: "cpu {{pod}}",
      B: "mem {{pod}}",
    }
    expect(
      firstNumberSeries([a, b], (f) => legends[f.ref]).map((s) => [
        s.label,
        s.key,
      ])
    ).toEqual([
      ["cpu x", "A:{pod=x}#value"],
      ["mem x", "B:{pod=x}#value"],
    ])
  })
})

describe("firstNumberSeries label collisions", () => {
  const value = (ref: string, labels?: Record<string, string>) =>
    frame({
      ref,
      labels,
      fields: [time, { name: "value", type: "number", values: [1] }],
    })
  const labelsOf = (
    frames: WireFrame[],
    legends: Record<string, string | undefined> = {}
  ) => firstNumberSeries(frames, (f) => legends[f.ref]).map((s) => s.label)

  it("prefixes identical label sets from two queries", () => {
    expect(
      labelsOf([value("A", { pod: "x" }), value("B", { pod: "x" })])
    ).toEqual(['A: {pod="x"}', 'B: {pod="x"}'])
  })

  it("leaves a shared templated legend alone when it resolves differently", () => {
    expect(
      labelsOf([value("A", { pod: "x" }), value("B", { pod: "y" })], {
        A: "{{pod}}",
        B: "{{pod}}",
      })
    ).toEqual(["x", "y"])
  })

  it("prefixes a shared literal legend", () => {
    expect(
      labelsOf([value("A", { pod: "x" }), value("B", { pod: "y" })], {
        A: "total",
        B: "total",
      })
    ).toEqual(["A: total", "B: total"])
  })

  it("prefixes unlabeled fallbacks from two queries", () => {
    expect(labelsOf([value("A"), value("B")])).toEqual(["A: value", "B: value"])
  })

  it("leaves collisions inside one query alone", () => {
    const a = value("A", { pod: "x" })
    expect(labelsOf([a, a])).toEqual(['{pod="x"}', '{pod="x"}'])
  })

  it("only prefixes the colliding labels", () => {
    expect(
      labelsOf([value("A", { pod: "x" }), value("B", { pod: "x" }), value("C")])
    ).toEqual(['A: {pod="x"}', 'B: {pod="x"}', "value"])
  })
})

describe("seriesKey", () => {
  it("starts with the frame's query ref", () => {
    const f = frame({
      ref: "errors",
      name: "up",
      labels: { b: "2", a: "1" },
      fields: [],
    })
    expect(seriesKey(f, "value")).toBe("errors:up{a=1,b=2}#value")
  })
})

describe("uniqueKey", () => {
  const take = (names: string[]) => {
    const seen = new Map<string, number>()
    return names.map((name) => uniqueKey(seen, name))
  }

  it("suffixes repeated names in order", () => {
    expect(take(["x", "x", "x"])).toEqual(["x", "x#1", "x#2"])
  })

  it("skips a suffixed name a real field already took", () => {
    expect(take(["x#1", "x", "x"])).toEqual(["x#1", "x", "x#2"])
    expect(take(["x", "x#1", "x"])).toEqual(["x", "x#1", "x#2"])
    expect(take(["x", "x", "x#1"])).toEqual(["x", "x#1", "x#1#1"])
  })

  it("never returns the same name twice", () => {
    const names = take(["x", "x#1", "x", "x#1", "x", "x#2", "x"])
    expect(new Set(names).size).toBe(names.length)
  })
})

describe("legendFor", () => {
  const panel = {
    id: "p",
    type: "timeseries",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    queries: [
      { query: "a", legend: "first" },
      { ref: "errors", query: "b", legend: "second" },
      { query: "c" },
    ],
  } as unknown as DataPanel

  it("matches a default ref by position", () => {
    expect(legendFor(panel, frame({ ref: "A", fields: [] }))).toBe("first")
    expect(legendFor(panel, frame({ ref: "C", fields: [] }))).toBeUndefined()
  })

  it("matches an explicit ref", () => {
    expect(legendFor(panel, frame({ ref: "errors", fields: [] }))).toBe(
      "second"
    )
  })

  it("has no legend for an unknown ref", () => {
    expect(legendFor(panel, frame({ ref: "Z", fields: [] }))).toBeUndefined()
    expect(legendFor(panel, frame({ ref: "B", fields: [] }))).toBeUndefined()
  })
})
