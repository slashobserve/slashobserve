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

function filter(
  key: string,
  op: AdhocFilter["op"],
  value: string
): AdhocFilter {
  return { key, op, value }
}

function orFilter(
  key: string,
  op: AdhocFilter["op"],
  value: string
): AdhocFilter {
  return { key, op, value, condition: "or" }
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
    expect(rows(filterByAdhoc([table], [[f]]), "pod")).toEqual(pods)
  })

  it("keeps every field aligned on the surviving rows", () => {
    const out = filterByAdhoc([table], [[filter("code", "=", "200")]])
    expect(rows(out, "pod")).toEqual(["a-1", "b-1"])
    expect(rows(out, "code")).toEqual([200, 200])
    expect(rows(out, "ok")).toEqual([true, true])
  })

  it("requires every filter to pass", () => {
    const out = filterByAdhoc(
      [table],
      [[filter("code", "=", "200"), filter("pod", "=~", "a-.*")]]
    )
    expect(rows(out, "pod")).toEqual(["a-1"])
  })

  it("matches nothing for an invalid regex, and excludes nothing with its negation", () => {
    expect(
      rows(filterByAdhoc([table], [[filter("pod", "=~", "(")]]), "pod")
    ).toEqual([])
    expect(
      rows(filterByAdhoc([table], [[filter("pod", "!~", "(")]]), "pod")
    ).toEqual(["a-1", "a-2", "b-1", null])
  })

  it("keeps a frame that has no field named by the key", () => {
    const other: PreparedFrame = {
      ref: "B",
      fields: [{ name: "node", type: "string", values: ["n1", "n2"] }],
    }
    const out = filterByAdhoc([table, other], [[filter("pod", "=", "a-1")]])
    expect(out[1]).toBe(other)
    expect(out[0].fields[0].values).toEqual(["a-1"])
  })

  it("applies only the filters whose key a frame has", () => {
    const out = filterByAdhoc(
      [table],
      [[filter("pod", "=", "a-2"), filter("node", "=", "n1")]]
    )
    expect(rows(out, "pod")).toEqual(["a-2"])
  })

  it("returns the frames untouched without filters", () => {
    const frames = [table]
    expect(filterByAdhoc(frames, [])).toBe(frames)
  })

  it("does not change its input", () => {
    const before = JSON.stringify(table)
    filterByAdhoc([table], [[filter("pod", "=", "a-1")]])
    expect(JSON.stringify(table)).toBe(before)
  })
})

describe("filterByAdhoc ordering operators", () => {
  const readings: PreparedFrame = {
    ref: "A",
    fields: [
      { name: "n", type: "string", values: ["2", "10", "9", "abc", null] },
    ],
  }

  it.each([
    ["less than a number compares numbers", filter("n", "<", "5"), ["2"]],
    [
      "greater than a number compares numbers and text for the rest",
      filter("n", ">", "5"),
      ["10", "9", "abc"],
    ],
    [
      "less than text compares text",
      filter("n", "<", "b"),
      ["2", "10", "9", "abc"],
    ],
    ["greater than text compares text", filter("n", ">", "ab"), ["abc"]],
    [
      "a number followed by text is text",
      filter("n", ">", "5 x"),
      ["9", "abc"],
    ],
    [
      "a decimal and an exponent are numbers",
      filter("n", "<", "1e1"),
      ["2", "9"],
    ],
  ])("%s", (_name, f, expected) => {
    expect(rows(filterByAdhoc([readings], [[f]]), "n")).toEqual(expected)
  })

  it("never matches an empty cell, as a null never compares in the query", () => {
    expect(
      rows(filterByAdhoc([readings], [[filter("n", "<", "z")]]), "n")
    ).not.toContain(null)
  })

  it("reads a number field as its text", () => {
    expect(
      rows(filterByAdhoc([table], [[filter("code", ">", "299")]]), "code")
    ).toEqual([500, 404])
  })
})

describe("filterByAdhoc groups and scopes", () => {
  it("joins groups with or, so a, b, or c is (a and b) or c", () => {
    const out = filterByAdhoc(
      [table],
      [
        [
          filter("pod", "=~", "a-.*"),
          filter("code", "=", "500"),
          orFilter("pod", "=", "b-1"),
        ],
      ]
    )
    expect(rows(out, "pod")).toEqual(["a-2", "b-1"])
  })

  it("ands the scopes as wholes, so base filters narrow every group", () => {
    const base = [filter("code", "=", "200")]
    const selected = [filter("pod", "=", "a-2"), orFilter("pod", "=", "b-1")]
    expect(rows(filterByAdhoc([table], [base, selected]), "pod")).toEqual([
      "b-1",
    ])
  })

  it("lets a group with no field of the frame pass every row of its scope", () => {
    const out = filterByAdhoc(
      [table],
      [[filter("pod", "=", "a-1"), orFilter("node", "=", "n1")]]
    )
    expect(rows(out, "pod")).toEqual(["a-1", "a-2", "b-1", null])
  })

  it("skips a scope none of whose fields the frame has and applies the other", () => {
    const out = filterByAdhoc(
      [table],
      [[filter("node", "=", "n1")], [filter("code", "=", "404")]]
    )
    expect(rows(out, "pod")).toEqual([null])
  })
})

describe("adhocFiltersOf", () => {
  const variables = [
    {
      type: "adhoc",
      name: "first",
      datasource: "prom",
      baseFilters: [
        { key: "env", operator: "=", value: "prod" },
        { key: "tier", operator: ">", value: "1", condition: "or" },
      ],
    },
    { type: "textbox", name: "text" },
    { type: "adhoc", name: "second", datasource: "ch" },
    { type: "adhoc", name: "empty", datasource: "prom" },
  ] as Variable[]

  it("gives each adhoc variable a scope for its base filters and one for its selection, and ignores other variables", () => {
    expect(
      adhocFiltersOf(variables, {
        first: ["pod|=|a", "|code|!~|5.."],
        text: ["pod|=|ignored"],
        second: ["host|=~|h.*"],
        empty: [""],
      })
    ).toEqual([
      [filter("env", "=", "prod"), orFilter("tier", ">", "1")],
      [filter("pod", "=", "a"), orFilter("code", "!~", "5..")],
      [filter("host", "=~", "h.*")],
    ])
  })

  it("keeps base filters when nothing is selected", () => {
    expect(adhocFiltersOf(variables, {})).toEqual([
      [filter("env", "=", "prod"), orFilter("tier", ">", "1")],
    ])
  })

  it("skips text that is not a filter and variables with no selection", () => {
    expect(
      adhocFiltersOf(variables, { second: ["nonsense", "x|~|1"] })
    ).toEqual([[filter("env", "=", "prod"), orFilter("tier", ">", "1")]])
  })
})
