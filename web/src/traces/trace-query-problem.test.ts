import { describe, expect, it } from "vitest"

import { traceQLProblem } from "./trace-query-problem"

describe("traceQLProblem", () => {
  it("strips the doubled prefix, names the end of the query and marks the position", () => {
    expect(
      traceQLProblem(
        "invalid query: invalid TraceQL query: parse error at line 1, col 9: syntax error: unexpected $end",
        "{ name = "
      )
    ).toBe(
      'Invalid TraceQL at line 1, column 9 after "{ name =": unexpected end of the query.'
    )
  })

  it("keeps an unexpected token as written", () => {
    expect(
      traceQLProblem(
        "invalid TraceQL query: parse error at line 1, col 3: syntax error: unexpected IDENTIFIER",
        "{ x }"
      )
    ).toBe(
      'Invalid TraceQL at line 1, column 3 after "{": unexpected IDENTIFIER.'
    )
  })

  it("shortens a long excerpt and counts lines", () => {
    const query = `{ resource.service.name = "checkout" &&\n  span.http.route = }`
    const text = traceQLProblem(
      "parse error at line 2, col 20: syntax error: unexpected }",
      query
    )
    expect(text).toContain("line 2, column 20")
    expect(text).toContain('after "...')
  })

  it("only removes the doubled prefix from another failure", () => {
    expect(
      traceQLProblem("invalid query: invalid TraceQL query: boom", "{}")
    ).toBe("invalid TraceQL query: boom")
    expect(traceQLProblem("tempo is unavailable", "{}")).toBe(
      "tempo is unavailable"
    )
  })
})
