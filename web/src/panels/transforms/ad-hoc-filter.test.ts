import { describe, expect, it } from "vitest"

import type { AdhocFilter } from "@/dashboards/variable-controls/adhoc"
import type { Variable } from "@/spec"

import { adhocFiltersOf, filterByAdhoc } from "./ad-hoc-filter"
import type { PreparedFrame } from "./names"

const table: PreparedFrame = {
  ref: "A",
  fields: [
    { name: "pod", type: "string", values: ["a-1", "a-2", "b-1", null] },
    { name: "code", type: "number", values: [200, 500, 200, 404] },
    { name: "ok", type: "bool", values: [true, false, true, false] },
  ],
}

function rows(frames: PreparedFrame[], field: string): unknown[] {
  return frames[0].fields.find((f) => f.name === field)?.values ?? []
}

function filter(key: string, op: AdhocFilter["op"], value: string) {
  return { key, op, value }
}

describe("filterByAdhoc", () => {
  it.each([
    ["equals", filter("pod", "=", "a-1"), ["a-1"]],
    ["differs", filter("pod", "!=", "a-1"), ["a-2", "b-1", null]],
    ["matches an anchored regex", filter("pod", "=~", "a-.*"), ["a-1", "a-2"]],
    ["does not match the middle of a value", filter("pod", "=~", "a"), []],
    ["excludes a regex", filter("pod", "!~", "a-.*"), ["b-1", null]],
    ["reads a number as its text", filter("code", "=", "500"), ["a-2"]],
    ["reads a boolean as its text", filter("ok", "=", "true"), ["a-1", "b-1"]],
    ["reads a null as empty text", filter("pod", "=", ""), [null]],
  ])("keeps the rows that pass: %s", (_name, f, pods) => {
    expect(rows(filterByAdhoc([table], [f]), "pod")).toEqual(pods)
  })

  it("keeps every field aligned on the surviving rows", () => {
    const out = filterByAdhoc([table], [filter("code", "=", "200")])
    expect(rows(out, "pod")).toEqual(["a-1", "b-1"])
    expect(rows(out, "code")).toEqual([200, 200])
    expect(rows(out, "ok")).toEqual([true, true])
  })

  it("requires every filter to pass", () => {
    const out = filterByAdhoc(
      [table],
      [filter("code", "=", "200"), filter("pod", "=~", "a-.*")]
    )
    expect(rows(out, "pod")).toEqual(["a-1"])
  })

  it("matches nothing for an invalid regex, and excludes nothing with its negation", () => {
    expect(
      rows(filterByAdhoc([table], [filter("pod", "=~", "(")]), "pod")
    ).toEqual([])
    expect(
      rows(filterByAdhoc([table], [filter("pod", "!~", "(")]), "pod")
    ).toEqual(["a-1", "a-2", "b-1", null])
  })

  it("keeps a frame that has no field named by the key", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [{ name: "node", type: "string", values: ["n1", "n2"] }],
    }
    const out = filterByAdhoc([table, other], [filter("pod", "=", "a-1")])
    expect(out[1]).toBe(other)
    expect(out[0].fields[0].values).toEqual(["a-1"])
  })

  it("applies only the filters whose key a frame has", () => {
    const out = filterByAdhoc(
      [table],
      [filter("pod", "=", "a-2"), filter("node", "=", "n1")]
    )
    expect(rows(out, "pod")).toEqual(["a-2"])
  })

  it("returns the frames untouched without filters", () => {
    const frames = [table]
    expect(filterByAdhoc(frames, [])).toBe(frames)
  })

  it("does not change its input", () => {
    const before = JSON.stringify(table)
    filterByAdhoc([table], [filter("pod", "=", "a-1")])
    expect(JSON.stringify(table)).toBe(before)
  })
})

describe("adhocFiltersOf", () => {
  const variables = [
    { type: "adhoc", name: "first", datasource: "prom" },
    { type: "textbox", name: "text" },
    { type: "adhoc", name: "second", datasource: "ch" },
    { type: "adhoc", name: "empty", datasource: "prom" },
  ] as Variable[]

  it("collects the filters of every adhoc variable and ignores other variables", () => {
    expect(
      adhocFiltersOf(variables, {
        first: ["pod|=|a", "code|!~|5.."],
        text: ["pod|=|ignored"],
        second: ["host|=~|h.*"],
        empty: [""],
      })
    ).toEqual([
      { key: "pod", op: "=", value: "a" },
      { key: "code", op: "!~", value: "5.." },
      { key: "host", op: "=~", value: "h.*" },
    ])
  })

  it("skips text that is not a filter and variables with no selection", () => {
    expect(adhocFiltersOf(variables, { first: ["nonsense", "x|<|1"] })).toEqual(
      []
    )
  })
})
