import { describe, expect, test } from "vitest"

import { stringifySearch } from "@/lib/search"
import { validateLogsSearch } from "@/logs/search"

import { auditDatasourceUid } from "./audit-source"
import { auditInLogsLink } from "./open-in-logs"

describe("auditInLogsLink", () => {
  test("opens Logs on the audit datasource with the query and the range", () => {
    expect(
      auditInLogsLink({ q: "outcome:denied", from: "now-7d", to: "now" })
    ).toEqual({
      to: "/logs/$datasourceUid",
      params: { datasourceUid: auditDatasourceUid },
      search: { q: "outcome:denied", from: "now-7d", to: "now" },
    })
  })

  test("keeps the default range, which Logs would otherwise read as one hour", () => {
    const { search } = auditInLogsLink({ q: "", from: "now-24h", to: "now" })
    expect(search).toEqual({ q: undefined, from: "now-24h", to: "now" })
  })

  test("keeps Lucene and Query DSL text that Logs reads on this datasource", () => {
    const dsl = '{"query":{"term":{"outcome":"denied"}}}'
    for (const q of ["lucene:outcome:denied", dsl]) {
      const { search } = auditInLogsLink({ q, from: "now-1h", to: "now" })
      expect(validateLogsSearch(search).q).toBe(q)
    }
  })

  test("survives the address round trip with absolute times", () => {
    const { search } = auditInLogsLink({
      q: 'actor:"Ada Lovelace" and action:dashboard.*',
      from: "2026-11-20T00:00:00.000Z",
      to: "2026-11-21T00:00:00.000Z",
    })
    const address = stringifySearch(search)
    const params = Object.fromEntries(new URLSearchParams(address))
    expect(validateLogsSearch(params)).toMatchObject({
      q: 'actor:"Ada Lovelace" and action:dashboard.*',
      from: "2026-11-20T00:00:00.000Z",
      to: "2026-11-21T00:00:00.000Z",
    })
  })

  test("leaves out a query that Logs cannot take", () => {
    const { search } = auditInLogsLink({
      q: "a".repeat(8001),
      from: "now-1h",
      to: "now",
    })
    expect(search.q).toBeUndefined()
  })
})
