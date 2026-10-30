import { describe, expect, it } from "vitest"

import {
  clearFilter,
  distinctValues,
  filterRows,
  maxFilterValues,
  toggleValue,
} from "./column-filter"
import type { TableRow } from "./table-model"

function rows(values: Record<string, unknown>[]): TableRow[] {
  return values.map((entry, index) => ({
    key: String(index),
    values: entry,
  })) as TableRow[]
}

const sample = rows([
  { pod: "b", code: 200 },
  { pod: "a", code: 500 },
  { pod: "b", code: 500 },
  { pod: "a", code: 200 },
])

const textOf = (key: string, row: TableRow) => String(row.values[key])

describe("distinctValues", () => {
  it("lists each value once in natural order", () => {
    const some = rows([{ n: "10" }, { n: "9" }, { n: "10" }, { n: "x" }])
    expect(distinctValues(some, (row) => String(row.values.n))).toEqual({
      values: ["9", "10", "x"],
      total: 3,
    })
  })

  it("caps the list but reports the true count", () => {
    const many = rows(
      Array.from({ length: maxFilterValues + 5 }, (_, n) => ({ n }))
    )
    const result = distinctValues(many, (row) => String(row.values.n))
    expect(result.values).toHaveLength(maxFilterValues)
    expect(result.total).toBe(maxFilterValues + 5)
  })
})

describe("filterRows", () => {
  it("keeps every row when no filter holds a value", () => {
    expect(filterRows(sample, new Map(), textOf)).toBe(sample)
    expect(
      filterRows(sample, new Map([["pod", new Set<string>()]]), textOf)
    ).toBe(sample)
  })

  it("keeps rows whose text is in the kept set", () => {
    const kept = filterRows(sample, new Map([["pod", new Set(["a"])]]), textOf)
    expect(kept.map((row) => row.key)).toEqual(["1", "3"])
  })

  it("requires every filtered column to match", () => {
    const kept = filterRows(
      sample,
      new Map([
        ["pod", new Set(["a", "b"])],
        ["code", new Set(["500"])],
      ]),
      textOf
    )
    expect(kept.map((row) => row.key)).toEqual(["1", "2"])
  })
})

describe("toggleValue and clearFilter", () => {
  it("adds and removes a value without changing the input", () => {
    const empty = new Map<string, ReadonlySet<string>>()
    const one = toggleValue(empty, "pod", "a")
    expect([...(one.get("pod") ?? [])]).toEqual(["a"])
    expect(empty.size).toBe(0)
    const two = toggleValue(one, "pod", "b")
    expect([...(two.get("pod") ?? [])]).toEqual(["a", "b"])
    expect([...(one.get("pod") ?? [])]).toEqual(["a"])
  })

  it("drops a column once its last value is unchecked", () => {
    const one = toggleValue(new Map(), "pod", "a")
    expect(toggleValue(one, "pod", "a").has("pod")).toBe(false)
  })

  it("clears one column and keeps the others", () => {
    const both = toggleValue(toggleValue(new Map(), "pod", "a"), "code", "200")
    const cleared = clearFilter(both, "pod")
    expect([...cleared.keys()]).toEqual(["code"])
  })
})
