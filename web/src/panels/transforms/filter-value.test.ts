import { describe, expect, it } from "vitest"

import { filterByValue } from "./filter-value"
import type { PreparedFrame } from "./names"

type Options = Parameters<typeof filterByValue>[1]
type Filter = Options["filters"][number]

const frame: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "host", type: "string", values: ["a1", "a2", "b1", null, "c1"] },
    { name: "load", type: "number", values: [1, 5, 10, 7, null] },
  ],
}

function hosts(
  filters: Filter[],
  extra: Partial<Options> = {}
): { host: unknown[]; load: unknown[] } {
  const [out] = filterByValue([frame], {
    type: "filterByValue",
    filters,
    ...extra,
  })
  return { host: out.fields[0].values, load: out.fields[1].values }
}

describe("filterByValue", () => {
  it.each<[string, Filter, unknown[]]>([
    ["gt", { field: "load", op: "gt", value: 5 }, ["b1", null]],
    ["gte", { field: "load", op: "gte", value: 5 }, ["a2", "b1", null]],
    ["lt", { field: "load", op: "lt", value: 5 }, ["a1"]],
    ["lte", { field: "load", op: "lte", value: 5 }, ["a1", "a2"]],
    ["eq number", { field: "load", op: "eq", value: 5 }, ["a2"]],
    ["eq text", { field: "host", op: "eq", value: "b1" }, ["b1"]],
    [
      "neq",
      { field: "host", op: "neq", value: "b1" },
      ["a1", "a2", null, "c1"],
    ],
    ["null", { field: "load", op: "null" }, ["c1"]],
    ["notNull", { field: "host", op: "notNull" }, ["a1", "a2", "b1", "c1"]],
    ["regex", { field: "host", op: "regex", value: "a.*" }, ["a1", "a2"]],
    ["range", { field: "load", op: "range", from: 5, to: 7 }, ["a2", null]],
    ["open range", { field: "load", op: "range", from: 7 }, ["b1", null]],
  ])("keeps the rows that pass %s", (_name, filter, expected) => {
    expect(hosts([filter]).host).toEqual(expected)
  })

  it("filters every field by the same rows", () => {
    expect(hosts([{ field: "load", op: "gte", value: 7 }])).toEqual({
      host: ["b1", null],
      load: [10, 7],
    })
  })

  it("drops the matching rows with keep exclude", () => {
    expect(
      hosts([{ field: "host", op: "regex", value: "a.*" }], { keep: "exclude" })
        .host
    ).toEqual(["b1", null, "c1"])
  })

  it("combines filters with any by default and all on request", () => {
    const filters: Filter[] = [
      { field: "host", op: "eq", value: "a1" },
      { field: "load", op: "gt", value: 6 },
    ]
    expect(hosts(filters).host).toEqual(["a1", "b1", null])
    expect(hosts(filters, { match: "all" }).host).toEqual([])
    expect(
      hosts(
        [
          { field: "load", op: "gt", value: 0 },
          { field: "load", op: "lt", value: 6 },
        ],
        { match: "all" }
      ).host
    ).toEqual(["a1", "a2"])
  })

  it("compares numbers written as text and refuses text that is not a number", () => {
    expect(hosts([{ field: "load", op: "gt", value: "6" }]).host).toEqual([
      "b1",
      null,
    ])
    expect(hosts([{ field: "load", op: "gt", value: "x" }]).host).toEqual([])
  })

  it("ignores a filter on an unknown field", () => {
    expect(hosts([{ field: "nope", op: "eq", value: 1 }]).host).toEqual(
      frame.fields[0].values
    )
    expect(
      hosts([
        { field: "nope", op: "eq", value: 1 },
        { field: "load", op: "lt", value: 2 },
      ]).host
    ).toEqual(["a1"])
  })

  it("keeps the input frame as it was", () => {
    hosts([{ field: "load", op: "gt", value: 5 }])
    expect(frame.fields[1].values).toEqual([1, 5, 10, 7, null])
  })
})
