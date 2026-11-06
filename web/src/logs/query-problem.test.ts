import { describe, expect, it } from "vitest"

import { parseKql } from "./kql"
import { problemOf } from "./query-problem"

function problem(text: string, offset = 0) {
  return problemOf(text, parseKql(text.slice(offset)), offset)
}

describe("problemOf", () => {
  it("has nothing to say about a query that parses", () => {
    expect(problem("level:error")).toBeNull()
  })

  it("points at the open group and closes it before the next and", () => {
    const text =
      "service.name : checkout and log.level : (error or warn and message : payment"
    const found = problem(text)
    expect(found).toMatchObject({
      message: 'expected ")"',
      start: 67,
      end: 68,
      column: 68,
      nothingRan: true,
      luceneSwitch: false,
      fix: {
        label: "Apply the suggested fix",
        text: "service.name : checkout and log.level : (error or warn) and message : payment",
      },
    })
    expect(found?.caret).toBe(`${" ".repeat(67)}^ expected ) here`)
    expect(found?.explanation).toBe(
      "The group that starts at (error is never closed. KQL groups need a closing parenthesis before the next and."
    )
  })

  it("closes the group at the end when no and follows", () => {
    expect(problem("log.level : (error or warn")?.fix?.text).toBe(
      "log.level : (error or warn)"
    )
  })

  it("keeps an and inside quotes and nested parentheses out of the fix", () => {
    expect(problem('a:(x or "p and q" or (r and s) and t')?.fix?.text).toBe(
      'a:(x or "p and q" or (r and s)) and t'
    )
  })

  it("offers no fix for an error it cannot repair", () => {
    const found = problem("level:")
    expect(found?.message).toBe('expected a value after ":"')
    expect(found?.fix).toBeNull()
    expect(found?.caret).toBe("     ^ expected a value after : here")
  })

  it("offers the Lucene switch and a KQL rewrite that also closes the group", () => {
    const found = problem("+level:error -message:(payment")
    expect(found?.luceneSwitch).toBe(true)
    expect(found?.fix).toEqual({
      label: "Rewrite as KQL",
      text: "level:error not message:(payment)",
    })
  })

  it("rewrites a Lucene operator even when the group stays open", () => {
    const found = problem("level:error~2 and (host:a b:(c")
    expect(found?.luceneSwitch).toBe(true)
    expect(found?.fix?.label).toBe("Rewrite as KQL")
    expect(found?.fix?.text.startsWith("level:error and")).toBe(true)
  })

  it("puts the caret under the line that holds the error", () => {
    const found = problem("a:1\nand b:(c")
    expect(found).toMatchObject({ line: 2, column: 7, start: 10 })
    expect(found?.caret).toBe("      ^ expected ) here")
  })

  it("counts offsets from the whole text when a prefix precedes the body", () => {
    const found = problem("kql: a:(b", 5)
    expect(found).toMatchObject({ start: 7, end: 8 })
    expect(found?.fix?.text).toBe("kql: a:(b)")
  })

  it.each([
    ["a)", "has no opening parenthesis"],
    ["items:{ name:x }", "Query DSL"],
    ["*name:x", "log.*"],
    ["message:a\\ b", "double quotes"],
    ['a:"x', "closing double quote"],
    ["not", "not negates"],
  ])("explains %s", (text, words) => {
    expect(problem(text)?.explanation).toContain(words)
  })
})
