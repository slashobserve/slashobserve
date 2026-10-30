import { describe, expect, it } from "vitest"

import { createFieldResolver } from "@/panels/field-config"
import type { WireField } from "@/panels/frames"
import type { PreparedFrame } from "@/panels/transforms"
import type { DataPanel } from "@/spec"

import {
  reduceItems,
  reduceOptionsOf,
  type ReduceOptions,
} from "./reduce-items"

function panel(extra: Partial<DataPanel> = {}): DataPanel {
  return {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 6, h: 4 },
    type: "stat",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
}

const rowTimes = [
  new Date(2025, 0, 2, 3, 4, 5).getTime(),
  new Date(2025, 0, 2, 3, 5, 5).getTime(),
  new Date(2025, 0, 2, 3, 6, 5).getTime(),
]

const time: WireField = { name: "Time", type: "time", values: rowTimes }

function frame(fields: WireField[]) {
  return { ref: "A", fields } as PreparedFrame
}

function items(
  frames: PreparedFrame[],
  options: Partial<ReduceOptions> = {},
  extra: Partial<DataPanel> = {}
) {
  const resolver = createFieldResolver(panel(extra))
  return reduceItems(frames, resolver, {
    reduce: "lastNotNull",
    allValues: false,
    limit: 5000,
    ...options,
  })
}

const cpu: WireField = { name: "cpu", type: "number", values: [1, 2, 3] }
const mem: WireField = { name: "mem", type: "number", values: [10, 20, 30] }
const host: WireField = {
  name: "host",
  type: "string",
  values: ["a", "b", "c"],
}

describe("reduceItems", () => {
  it("reduces each number field to one item by default", () => {
    const result = items([frame([time, cpu, host, mem])])
    expect(result.map((item) => [item.field.name, item.value])).toEqual([
      ["cpu", 3],
      ["mem", 30],
    ])
  })

  it("uses the chosen reducer", () => {
    const result = items([frame([time, cpu])], { reduce: "sum" })
    expect(result.map((item) => item.value)).toEqual([6])
  })

  it("keeps same-named fields on distinct keys", () => {
    const result = items([frame([time, cpu]), frame([time, cpu])])
    expect(new Set(result.map((item) => item.key)).size).toBe(2)
  })

  describe("fields", () => {
    const frames = [frame([time, cpu, host, mem])]

    it("takes a field by name", () => {
      const result = items(frames, { fields: "mem" })
      expect(result.map((item) => item.value)).toEqual([30])
    })

    it("takes the fields a pattern between slashes matches", () => {
      const result = items(frames, { fields: "/^c/" })
      expect(result.map((item) => item.field.name)).toEqual(["cpu"])
    })

    it("takes every field except time for /.*/ and keeps a text field's value", () => {
      const result = items(frames, { fields: "/.*/" })
      expect(result.map((item) => [item.field.name, item.raw])).toEqual([
        ["cpu", 3],
        ["host", "c"],
        ["mem", 30],
      ])
      expect(result[1].value).toBeNull()
    })

    it("matches a plain name only as a whole", () => {
      expect(items(frames, { fields: "cp" })).toEqual([])
    })

    it("matches nothing for an invalid pattern", () => {
      expect(items(frames, { fields: "/(/" })).toEqual([])
    })

    it("counts a text field with a count reducer", () => {
      const result = items(frames, { fields: "host", reduce: "count" })
      expect(result[0].value).toBe(3)
    })

    it("takes the first text value for a first reducer", () => {
      const result = items(frames, { fields: "host", reduce: "first" })
      expect(result[0].raw).toBe("a")
    })
  })

  describe("allValues", () => {
    it("makes one item per row, labelled by the first text field", () => {
      const result = items([frame([time, host, cpu])], { allValues: true })
      expect(result.map((item) => [item.label, item.value, item.row])).toEqual([
        ["a", 1, 0],
        ["b", 2, 1],
        ["c", 3, 2],
      ])
    })

    it("labels a row by its time when there is no text field", () => {
      const result = items([frame([time, cpu])], { allValues: true })
      expect(result.map((item) => item.label)).toEqual([
        "2025-01-02 03:04:05",
        "2025-01-02 03:05:05",
        "2025-01-02 03:06:05",
      ])
    })

    it("labels a row by its number when there is neither", () => {
      const result = items([frame([cpu])], { allValues: true })
      expect(result.map((item) => item.label)).toEqual(["1", "2", "3"])
    })

    it("stops at the row limit for each field", () => {
      const result = items([frame([time, cpu, mem])], {
        allValues: true,
        limit: 2,
      })
      expect(result.map((item) => item.value)).toEqual([1, 2, 10, 20])
    })

    it("keeps a missing value as an item without a value", () => {
      const sparse: WireField = {
        name: "cpu",
        type: "number",
        values: [1, null, 3],
      }
      const result = items([frame([time, sparse])], { allValues: true })
      expect(result.map((item) => item.value)).toEqual([1, null, 3])
    })

    it("gives every row its own key", () => {
      const result = items([frame([time, cpu]), frame([time, cpu])], {
        allValues: true,
      })
      expect(new Set(result.map((item) => item.key)).size).toBe(6)
    })

    it("shows only the picked fields", () => {
      const result = items([frame([time, host, cpu, mem])], {
        allValues: true,
        fields: "mem",
      })
      expect(result.map((item) => item.value)).toEqual([10, 20, 30])
    })
  })
})

describe("reduceOptionsOf", () => {
  it("fills the defaults for an empty display", () => {
    expect(reduceOptionsOf("gauge", {})).toEqual({
      reduce: "lastNotNull",
      allValues: false,
      limit: 5000,
      fields: undefined,
    })
  })

  it("keeps what the display sets", () => {
    expect(
      reduceOptionsOf("piechart", {
        reduce: "max",
        allValues: true,
        limit: 7,
        fields: "/^cpu/",
      })
    ).toEqual({ reduce: "max", allValues: true, limit: 7, fields: "/^cpu/" })
  })

  it("treats empty fields as absent", () => {
    expect(reduceOptionsOf("stat", { fields: "" }).fields).toBeUndefined()
  })
})
