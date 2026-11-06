import { describe, expect, it } from "vitest"

import {
  rowLabel,
  rowsOf,
  textOf,
  withoutField,
  type BuilderRow,
} from "./builder"

function row(partial: Partial<BuilderRow>): BuilderRow {
  return { id: "0", field: "", op: "is", value: "", values: [], ...partial }
}

describe("rowsOf", () => {
  it("reads the board's query as three chips and writes it back", () => {
    const text =
      "log.level : (error or warn) and message : payment and not host.name : canary-*"
    const rows = rowsOf(text)
    expect(rows?.map(rowLabel)).toEqual([
      "log.level : error or warn",
      "message : payment",
      "not host.name : canary-*",
    ])
    expect(textOf(rows ?? [])).toBe(text)
  })

  it("reads an empty query as no rows", () => {
    expect(rowsOf("  ")).toEqual([])
  })

  it("reads ranges, exists and free text", () => {
    const rows = rowsOf("latency >= 250 and trace.id:* and timeout")
    expect(rows?.map((r) => [r.field, r.op, r.value])).toEqual([
      ["latency", "gte", "250"],
      ["trace.id", "exists", ""],
      ["", "contains", "timeout"],
    ])
  })

  it("reads a quoted value the way the filter pills write it", () => {
    const rows = rowsOf('service.name:"checkout" and level:"error"')
    expect(rows?.map(rowLabel)).toEqual([
      "service.name : checkout",
      "level : error",
    ])
    expect(textOf(rows ?? [])).toBe("service.name : checkout and level : error")
  })

  it("keeps a value with spaces quoted", () => {
    const rows = rowsOf('message:"payment failed"')
    expect(rows?.[0].value).toBe("payment failed")
    expect(textOf(rows ?? [])).toBe('message : "payment failed"')
  })

  it.each([
    ["an or at the top", "a:1 or b:2"],
    ["a group", "a:1 and (b:2 or c:3)"],
    ["text that does not parse", "a:(b"],
    ["a wildcard inside a value list", "a:(x* or y)"],
    ["an and value list", "a:(x and y)"],
    ["a negated range", "not a > 1"],
    ["a negated free term", "not timeout"],
    ["a quoted star that a bare star would turn into a wildcard", 'a:"x*"'],
    ["an escaped star that a bare star would turn into exists", "a:\\*"],
  ])("returns null for %s", (_name, text) => {
    expect(rowsOf(text)).toBeNull()
  })
})

describe("textOf", () => {
  it("writes each operator", () => {
    const rows = [
      row({ field: "a", op: "is", value: "x y" }),
      row({ field: "b", op: "is not", value: "z" }),
      row({ field: "c", op: "is one of", values: ["p", "q r"] }),
      row({ field: "d", op: "exists" }),
      row({ field: "e", op: "lt", value: "2026-11-06T10:00" }),
      row({ op: "contains", value: "and" }),
    ]
    expect(textOf(rows)).toBe(
      'a : "x y" and not b : z and c : (p or "q r") and d : * and e < "2026-11-06T10:00" and "and"'
    )
  })

  it("writes nothing for no rows", () => {
    expect(textOf([])).toBe("")
  })

  it("writes a single row without an and", () => {
    expect(textOf([row({ field: "a", value: "b" })])).toBe("a : b")
  })
})

describe("withoutField", () => {
  it("drops every clause on the field", () => {
    expect(withoutField("a:1 and b:2 and a:3", "a")).toBe("b : 2")
  })

  it("gives nothing back when the builder cannot show the query", () => {
    expect(withoutField("a:1 or b:2", "a")).toBeNull()
  })
})
