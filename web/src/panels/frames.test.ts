import { describe, expect, it } from "vitest"

import type { WireFrame } from "@/dashboards/queries"

import {
  hasData,
  qualifyCollidingLabels,
  seriesKey,
  seriesLabel,
  uniqueKey,
} from "./frames"

const time: WireFrame["fields"][number] = {
  name: "time",
  type: "time",
  values: [1, 2, 3],
}

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

describe("qualifyCollidingLabels", () => {
  const labels = (items: { ref: string; label: string }[]) =>
    qualifyCollidingLabels(items).map((item) => item.label)

  it("prefixes identical labels from two queries with their refs", () => {
    expect(
      labels([
        { ref: "A", label: 'up{pod="x"}' },
        { ref: "B", label: 'up{pod="x"}' },
      ])
    ).toEqual(['A: up{pod="x"}', 'B: up{pod="x"}'])
  })

  it("leaves labels that collide inside one query alone", () => {
    expect(
      labels([
        { ref: "A", label: "up" },
        { ref: "A", label: "up" },
      ])
    ).toEqual(["up", "up"])
  })

  it("only prefixes the colliding labels", () => {
    expect(
      labels([
        { ref: "A", label: "total" },
        { ref: "B", label: "total" },
        { ref: "C", label: "value" },
      ])
    ).toEqual(["A: total", "B: total", "value"])
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
