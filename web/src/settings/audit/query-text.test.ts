import { describe, expect, test } from "vitest"

import {
  canEditTerms,
  requestQuery,
  runQuery,
  termState,
  toggleTerm,
} from "./query-text"

const actor = { name: "actor_name", alias: "actor" }
const outcome = { name: "outcome", alias: null }

describe("toggleTerm", () => {
  test("starts an empty query with the quoted term", () => {
    expect(toggleTerm("", outcome, "denied", false)).toBe('outcome:"denied"')
  })

  test("appends to an existing query with and", () => {
    expect(toggleTerm("origin:mcp", outcome, "denied", false)).toBe(
      'origin:mcp and outcome:"denied"'
    )
  })

  test("quotes and escapes a value with spaces and quotes", () => {
    expect(toggleTerm("", actor, 'Sam "the" Lee', false)).toBe(
      'actor_name:"Sam \\"the\\" Lee"'
    )
  })

  test("negates an excluded value", () => {
    expect(toggleTerm("origin:mcp", outcome, "denied", true)).toBe(
      'origin:mcp and not outcome:"denied"'
    )
  })

  test("removes a term that is already there", () => {
    expect(
      toggleTerm('origin:mcp and outcome:"denied"', outcome, "denied", false)
    ).toBe("origin : mcp")
    expect(toggleTerm("outcome:denied", outcome, "denied", false)).toBe("")
  })

  test("finds a term written with the field's alias", () => {
    expect(toggleTerm('actor:"Ada"', actor, "Ada", false)).toBe("")
  })

  test("flips an included term to excluded in place", () => {
    expect(
      toggleTerm("outcome:denied and origin:mcp", outcome, "denied", true)
    ).toBe('not outcome : "denied" and origin : mcp')
  })

  test("removes an excluded term when excluded again", () => {
    expect(
      toggleTerm("not outcome:denied and origin:mcp", outcome, "denied", true)
    ).toBe("origin : mcp")
  })

  test("wraps an or query before adding a term", () => {
    expect(
      toggleTerm("origin:mcp or origin:cli", outcome, "denied", false)
    ).toBe('(origin:mcp or origin:cli) and outcome:"denied"')
  })

  test("keeps the kql: prefix", () => {
    expect(toggleTerm("kql: origin:mcp", outcome, "denied", false)).toBe(
      'kql: origin:mcp and outcome:"denied"'
    )
  })

  test("leaves Lucene, Query DSL and unreadable text alone", () => {
    expect(toggleTerm("lucene: origin:mcp", outcome, "denied", false)).toBe(
      null
    )
    expect(toggleTerm('{"query":{}}', outcome, "denied", false)).toBe(null)
    expect(toggleTerm("origin:(mcp", outcome, "denied", false)).toBe(null)
    expect(canEditTerms("lucene: a")).toBe(false)
    expect(canEditTerms("")).toBe(true)
  })
})

describe("termState", () => {
  test("reads whether a value is included, excluded or absent", () => {
    const text = 'outcome:denied and not actor:"Ada"'
    expect(termState(text, outcome, "denied")).toBe("include")
    expect(termState(text, actor, "Ada")).toBe("exclude")
    expect(termState(text, outcome, "success")).toBe(null)
  })

  test("does not read a prefix match as the value", () => {
    expect(termState("outcome:den*", outcome, "denied")).toBe(null)
  })
})

describe("related queries", () => {
  test("quote the request and run ids", () => {
    expect(requestQuery("0193")).toBe('request_id:"0193"')
    expect(runQuery("5b1e")).toBe('run_id:"5b1e"')
  })
})
