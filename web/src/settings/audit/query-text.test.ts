import { describe, expect, test } from "vitest"

import {
  actorQuery,
  canEditTerms,
  requestQuery,
  runQuery,
  targetQuery,
  termState,
  toggleTermRemembering,
  toggleTerm,
} from "./query-text"
import type { AuditEvent } from "./stream"

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
    ).toBe("origin:mcp")
    expect(toggleTerm("outcome:denied", outcome, "denied", false)).toBe("")
  })

  test("removes every copy of a term written twice", () => {
    expect(
      toggleTerm(
        "outcome:denied and origin:mcp and outcome:denied",
        outcome,
        "denied",
        false
      )
    ).toBe("origin:mcp")
  })

  test("flips a term written twice into one copy", () => {
    expect(
      toggleTerm(
        "outcome:denied and origin:mcp and not outcome:denied",
        outcome,
        "denied",
        true
      )
    ).toBe('not outcome:"denied" and origin:mcp')
  })

  test("finds a term written with the field's alias", () => {
    expect(toggleTerm('actor:"Ada"', actor, "Ada", false)).toBe("")
  })

  test("flips an included term to excluded in place", () => {
    expect(
      toggleTerm("outcome:denied and origin:mcp", outcome, "denied", true)
    ).toBe('not outcome:"denied" and origin:mcp')
  })

  test("removes an excluded term when excluded again", () => {
    expect(
      toggleTerm("not outcome:denied and origin:mcp", outcome, "denied", true)
    ).toBe("origin:mcp")
  })

  test("wraps an or query before adding a term", () => {
    expect(
      toggleTerm("origin:mcp or origin:cli", outcome, "denied", false)
    ).toBe('(origin:mcp or origin:cli) and outcome:"denied"')
  })

  test("hands back the typed text byte for byte after a chip goes on and off", () => {
    const typed = [
      "actor:admin and not action:query.run",
      "actor : admin   and\n  not action:query.run",
      'origin:mcp and (action:a or action:b) and actor:"Ada Lovelace"',
      "kql:  origin:mcp",
      "origin:mcp",
    ]
    for (const text of typed) {
      const on = toggleTerm(text, outcome, "denied", false)
      expect(on).not.toBe(null)
      expect(toggleTerm(on as string, outcome, "denied", false)).toBe(text)
      const off = toggleTerm(text, outcome, "denied", true)
      expect(toggleTerm(off as string, outcome, "denied", true)).toBe(text)
    }
  })

  test("removes only the chip's clause from text typed with odd spacing", () => {
    const text = "actor:admin   and\n  not action:query.run  and origin:mcp"
    const action = { name: "action", alias: null }
    expect(toggleTerm(text, action, "query.run", true)).toBe(
      "actor:admin  and origin:mcp"
    )
    expect(
      toggleTerm(text, { name: "actor", alias: null }, "admin", false)
    ).toBe("not action:query.run  and origin:mcp")
  })

  test("finds a chip beside a group the person typed", () => {
    const text = '(origin:mcp or origin:cli) and outcome:"denied"'
    expect(termState(text, outcome, "denied")).toBe("include")
    expect(toggleTerm(text, outcome, "denied", false)).toBe(
      "(origin:mcp or origin:cli)"
    )
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

describe("actor and target filters", () => {
  const base = {
    actor_kind: "user",
    actor_id: "a1",
    actor_name: "Ada",
    target_kind: "dashboard",
    target_id: "d1",
  } as AuditEvent

  test("filter to the actor by id, then name, then kind", () => {
    expect(actorQuery(base)).toBe('actor_id:"a1"')
    expect(actorQuery({ ...base, actor_id: null })).toBe('actor_name:"Ada"')
    expect(
      actorQuery({
        ...base,
        actor_id: null,
        actor_name: null,
        actor_kind: "system",
      })
    ).toBe('actor_kind:"system"')
  })

  test("filter to the target by kind and id", () => {
    expect(targetQuery(base)).toBe('target:"dashboard/d1"')
    expect(targetQuery({ ...base, target_kind: null })).toBe('target_id:"d1"')
    expect(targetQuery({ ...base, target_id: null })).toBe(
      'target_kind:"dashboard"'
    )
    expect(targetQuery({ ...base, target_id: null, target_kind: null })).toBe(
      null
    )
  })
})

describe("toggleTermRemembering", () => {
  const typed = "origin:mcp or origin:cli"

  test("takes back the parentheses a chip added with the chip", () => {
    const on = toggleTermRemembering(typed, outcome, "denied", false, null)
    expect(on?.text).toBe('(origin:mcp or origin:cli) and outcome:"denied"')
    const off = toggleTermRemembering(
      on?.text as string,
      outcome,
      "denied",
      false,
      on?.memory ?? null
    )
    expect(off?.text).toBe(typed)
    expect(off?.memory).toBe(null)
  })

  test("keeps parentheses the person typed", () => {
    const text = "(origin:mcp or origin:cli)"
    const on = toggleTermRemembering(text, outcome, "denied", false, null)
    expect(on?.memory).toBe(null)
    expect(
      toggleTermRemembering(on?.text as string, outcome, "denied", false, null)
        ?.text
    ).toBe(text)
  })

  test("keeps the parentheses once the text was edited after the chip", () => {
    const on = toggleTermRemembering(typed, outcome, "denied", false, null)
    const edited = (on?.text as string) + " and actor:bob"
    const off = toggleTermRemembering(
      edited,
      outcome,
      "denied",
      false,
      on?.memory ?? null
    )
    expect(off?.text).toBe("(origin:mcp or origin:cli) and actor:bob")
  })

  test("does not remember anything for a query that needs no parentheses", () => {
    expect(
      toggleTermRemembering("origin:mcp", outcome, "denied", false, null)
        ?.memory
    ).toBe(null)
  })
})
