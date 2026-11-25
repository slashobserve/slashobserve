import { describe, expect, it } from "vitest"

import {
  canBuild,
  rowLabel,
  rowsOf,
  textOf,
  equivalentField,
  withRenamedField,
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

describe("withRenamedField", () => {
  it("renames the field of every clause that uses it", () => {
    expect(withRenamedField("a:1 and b:2 and a:3", "a", "x.a")).toBe(
      "x.a : 1 and b : 2 and x.a : 3"
    )
  })

  it("gives nothing back when the builder cannot show the query", () => {
    expect(withRenamedField("a:1 or b:2", "a", "x.a")).toBeNull()
  })
})

describe("equivalentField", () => {
  it("finds the one mapped field that ends with the unknown name", () => {
    expect(
      equivalentField("service.name", ["resource.service.name", "message"])
    ).toBe("resource.service.name")
  })

  it("finds the shorter mapped name inside a prefixed unknown one", () => {
    expect(
      equivalentField("resource.service.name", ["service.name", "message"])
    ).toBe("service.name")
  })

  it("finds the mapped field that differs only in capitals", () => {
    expect(equivalentField("Body", ["body", "message"])).toBe("body")
    expect(equivalentField("body", ["body", "message"])).toBeNull()
  })

  it("matches a dotted ending without regard to case", () => {
    expect(
      equivalentField("Service.Name", ["resource.service.name", "message"])
    ).toBe("resource.service.name")
    expect(equivalentField("resource.Service.name", ["service.NAME"])).toBe(
      "service.NAME"
    )
    expect(equivalentField("Name", ["a.name", "b.NAME"])).toBeNull()
  })

  it("ignores keyword shadows and stays silent when two fields fit", () => {
    expect(equivalentField("name", ["a.name", "b.name"])).toBeNull()
    expect(
      equivalentField("service.name", ["resource.service.name.keyword"])
    ).toBeNull()
  })
})

describe("canBuild", () => {
  it.each([
    ["", true],
    ["a:1 and b:2", true],
    ["a:1 or b:2", false],
    ["lucene:a:1", false],
    ["kql:a:1", false],
    ['{"query":{}}', false],
  ])("answers for %j", (text, want) => {
    expect(canBuild(text)).toBe(want)
  })
})
