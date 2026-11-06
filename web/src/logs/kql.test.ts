// @vitest-environment node
import { describe, expect, it } from "vitest"

import {
  appendExists,
  appendIn,
  appendKql,
  appendRange,
  escapeKql,
  flatClauses,
  kqlFields,
  kqlText,
  parseKql,
  type KqlNode,
} from "./kql"

type Case = {
  text: string
  ast?: string
  error?: string
  start?: number
  end?: number
}

const cases = (await (
  await fetch(
    new URL("../../../internal/kql/testdata/cases.json", import.meta.url)
  )
).json()) as Case[]

function flags(node: { quoted?: boolean; wildcard?: boolean }): string {
  return (node.wildcard ? " wildcard" : "") + (node.quoted ? " quoted" : "")
}

function render(node: KqlNode): string {
  switch (node.kind) {
    case "and":
    case "or":
      return `(${node.kind} ${node.items.map(render).join(" ")})`
    case "not":
      return `(not ${render(node.item)})`
    case "match":
      return `(match ${node.field} ${JSON.stringify(node.value)}${flags(node)})`
    case "range":
      return `(range ${node.field} ${node.op} ${JSON.stringify(node.value)}${flags(node)})`
    case "exists":
      return `(exists ${node.field})`
    case "free":
      return `(free ${JSON.stringify(node.value)}${flags(node)})`
    case "group":
      return `(group ${node.field === undefined ? "" : node.field + " "}${render(node.item)})`
    case "in":
      return `(in ${node.field} ${node.op} ${node.values.join(",")})`
  }
}

function withoutOffsets(ast: string): string {
  return ast.replace(/ \d+:\d+/g, "")
}

describe("parseKql against the server vectors", () => {
  const parsed = cases.filter((c) => c.ast !== undefined)
  const failing = cases.filter((c) => c.error !== undefined)

  it.each(parsed.map((c) => [c.text, c.ast] as const))(
    "reads %s with the server's tree",
    (text, ast) => {
      const { node, error } = parseKql(text)
      expect(error).toBeNull()
      expect(render(node as KqlNode)).toBe(withoutOffsets(ast as string))
    }
  )

  it.each(parsed.map((c) => c.text))(
    "prints %s as text that reads back to the same tree",
    (text) => {
      const first = parseKql(text).node as KqlNode
      const second = parseKql(kqlText(first)).node as KqlNode
      expect(render(second)).toBe(render(first))
    }
  )

  it.each(failing.map((c) => [c.text, c.error, c.start, c.end] as const))(
    "rejects %s with the server's message and offsets",
    (text, message, start, end) => {
      const { node, error } = parseKql(text)
      expect(node).toBeNull()
      expect(error).toEqual({ message, start, end })
    }
  )

  it.each(["", "   ", "\n\t"])("reads %j as an empty query", (text) => {
    expect(parseKql(text)).toEqual({ node: null, error: null })
  })

  it("refuses 21 levels of parentheses and accepts 20", () => {
    const wrap = (n: number) => "(".repeat(n) + "a" + ")".repeat(n)
    expect(parseKql(wrap(20)).error).toBeNull()
    expect(parseKql(wrap(21)).error).toEqual({
      message: "query too complex",
      start: 20,
      end: 21,
    })
  })

  it("refuses 200 terms joined by and and accepts 199", () => {
    const terms = (n: number) => Array(n).fill("a").join(" ")
    expect(parseKql(terms(199)).error).toBeNull()
    expect(parseKql(terms(200)).error).toEqual({
      message: "query too complex",
      start: 0,
      end: 399,
    })
  })
})

describe("kqlText", () => {
  it.each([
    ["log.level:(error or warn)", "log.level : (error or warn)"],
    ["a>=3", "a >= 3"],
    ["trace.id:*", "trace.id : *"],
    ["not host.name:canary-*", "not host.name : canary-*"],
    ['message:"payment failed"', 'message : "payment failed"'],
    ["a:\\*", "a : \\*"],
    ["(a or b) and c", "(a or b) and c"],
    ['@timestamp >= "2026-11-06T10:00"', '@timestamp >= "2026-11-06T10:00"'],
  ])("prints %s as %s", (text, want) => {
    expect(kqlText(parseKql(text).node as KqlNode)).toBe(want)
  })
})

describe("appendKql", () => {
  it.each([
    ["", "a", "x", false, 'a:"x"'],
    ["", "a", "x", true, 'not a:"x"'],
    ["a:1", "c", "d", false, 'a:1 and c:"d"'],
    ["a:1 and b:2", "c", "d", false, 'a:1 and b:2 and c:"d"'],
    ["a:1 or b:2", "c", "d", true, '(a:1 or b:2) and not c:"d"'],
    ["(a:1 or b:2)", "c", "d", false, '(a:1 or b:2) and c:"d"'],
    ["not a:1", "c", "d", false, 'not a:1 and c:"d"'],
    ["a:(1 or 2)", "c", "d", false, 'a:(1 or 2) and c:"d"'],
    ["  a:1  ", "c", "d", false, 'a:1 and c:"d"'],
    ["", "c", 'x"y\\z', false, 'c:"x\\"y\\\\z"'],
    ["a:", "c", "d", false, '(a:) and c:"d"'],
  ] as const)(
    "appends to %j the field %s and value %s (negate %s) as %s",
    (text, field, value, negate, want) => {
      expect(appendKql(text, field, value, negate)).toBe(want)
    }
  )

  it("adds exists, one-of and range clauses with the same wrapping rule", () => {
    expect(appendExists("a:1", "trace.id", false)).toBe("a:1 and trace.id:*")
    expect(appendExists("", "trace.id", true)).toBe("not trace.id:*")
    expect(appendIn("a:1 or b:2", "c", ["x", "y"], false)).toBe(
      '(a:1 or b:2) and c:("x" or "y")'
    )
    expect(appendIn("a:1", "c", [], false)).toBe("a:1")
    expect(appendRange("a:1", "latency", ">", "250")).toBe(
      "a:1 and latency > 250"
    )
    expect(appendRange("", "@timestamp", ">=", "2026-11-06T10:00")).toBe(
      '@timestamp >= "2026-11-06T10:00"'
    )
  })
})

describe("escapeKql", () => {
  it.each([
    ["plain", '"plain"'],
    ["", '""'],
    ['a"b', '"a\\"b"'],
    ["a\\b", '"a\\\\b"'],
    ['a\\"b', '"a\\\\\\"b"'],
    ["with space and *", '"with space and *"'],
  ])("escapes %j as %s", (value, want) => {
    expect(escapeKql(value)).toBe(want)
  })
})

describe("kqlFields", () => {
  it("lists each field once in order of appearance", () => {
    expect(
      kqlFields("a:1 and (b > 2 or not c:*) and a:2 and d:(x or y) and free")
    ).toEqual(["a", "b", "c", "d"])
  })

  it("lists nothing for text that does not parse", () => {
    expect(kqlFields("a:(")).toEqual([])
  })
})

describe("flatClauses", () => {
  function flat(text: string) {
    const node = parseKql(text).node as KqlNode
    return flatClauses(node)?.map(render) ?? null
  }

  it("returns the items of a top-level and", () => {
    expect(flat("a:1 and b > 2 and c:*")).toEqual([
      '(match a "1")',
      '(range b > "2")',
      "(exists c)",
    ])
  })

  it("reads a single clause, a value list and a negated clause", () => {
    expect(flat("a:(x or y)")).toEqual(["(in a or x,y)"])
    expect(flat("not a:x")).toEqual(['(not (match a "x"))'])
  })

  it.each([
    "a:1 and (b:2 or c:3)",
    "a:1 or b:2",
    "(a:1)",
    "a:(x* or y)",
    "not (a:1 and b:2)",
    "a:(x and not y)",
  ])("returns null for %s", (text) => {
    expect(flat(text)).toBeNull()
  })
})
