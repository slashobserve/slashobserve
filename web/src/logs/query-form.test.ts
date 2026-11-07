import { describe, expect, it } from "vitest"

import {
  formOf,
  kqlFromLucene,
  looksLikeLucene,
  splitForm,
  withForm,
} from "./query-form"

describe("formOf", () => {
  it.each([
    ["level:error", "kql"],
    ["lucene:level:error", "lucene"],
    ["LUCENE: level:error", "lucene"],
    ["  kql:level:error", "kql"],
    ['{"query":{"match_all":{}}}', "dsl"],
    ['  \n{"query":{}}', "dsl"],
    ["", "kql"],
  ] as const)("reads %j as %s", (text, form) => {
    expect(formOf(text)).toBe(form)
  })

  it("lets the default form decide an unprefixed query", () => {
    expect(formOf("level:error", "lucene")).toBe("lucene")
    expect(formOf("kql:level:error", "lucene")).toBe("kql")
  })
})

describe("splitForm", () => {
  it("returns the body without the prefix and where it starts", () => {
    expect(splitForm("  lucene:  a:b  ")).toEqual({
      form: "lucene",
      body: "a:b",
      offset: 11,
    })
    expect(splitForm("a:b")).toEqual({ form: "kql", body: "a:b", offset: 0 })
  })
})

describe("withForm", () => {
  it("adds, swaps and removes the prefix", () => {
    expect(withForm("level:error", "lucene")).toBe("lucene: level:error")
    expect(withForm("lucene:level:error", "kql")).toBe("level:error")
    expect(withForm("kql: level:error", "lucene")).toBe("lucene: level:error")
    expect(withForm("level:error", "kql")).toBe("level:error")
  })

  it("leaves a DSL body alone and starts an empty query from a skeleton", () => {
    expect(withForm('{"query":{}}', "kql")).toBe('{"query":{}}')
    expect(withForm("level:error", "dsl")).toBe("level:error")
    expect(formOf(withForm("", "dsl"))).toBe("dsl")
  })
})

describe("looksLikeLucene", () => {
  it.each([
    "+level:error -host:canary",
    "level:error -host:canary",
    "message:payment~2",
    '"payment failed"~3',
    "message:payment^2",
    "level:error AND host:web-1",
    "level:error OR host:web-1",
  ])("flags %s", (text) => {
    expect(looksLikeLucene(text)).toBe(true)
  })

  it.each([
    "level:error and host:web-1",
    "log.level : (error or warn) and message : payment",
    "host.name:web-1",
    "latency > 250",
  ])("does not flag %s", (text) => {
    expect(looksLikeLucene(text)).toBe(false)
  })
})

describe("kqlFromLucene", () => {
  it.each([
    ["+level:error -host:canary", "level:error not host:canary"],
    ["message:payment~2 AND level:error", "message:payment and level:error"],
    ['"payment failed"~2 OR timeout', '"payment failed" or timeout'],
    ["NOT level:debug", "not level:debug"],
    ['message:"a -b" -c', 'message:"a -b" not c'],
  ])("rewrites %s as %s", (text, want) => {
    expect(kqlFromLucene(text)).toBe(want)
  })

  it.each([
    ["a Lucene range", "status:[500 TO 599]"],
    ["a boost", "message:payment^2"],
    ["text already in KQL", "level:error and host:web-1"],
    ["a DSL body", '{"query":{}}'],
  ])("gives up on %s", (_name, text) => {
    expect(kqlFromLucene(text)).toBeNull()
  })
})
