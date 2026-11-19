import { describe, expect, it } from "vitest"

import { shorterRange, traceQLProblem } from "./trace-query-problem"

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

  it("keeps a message the server already cleaned and drops only its kind prefix", () => {
    const cleaned =
      'Invalid TraceQL at line 1, column 9 after "{ name =": unexpected end of the query.'
    expect(traceQLProblem(`invalid query: ${cleaned}`, "{ name = ")).toBe(
      cleaned
    )
    expect(traceQLProblem(cleaned, "{ name = ")).toBe(cleaned)
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

describe("shorterRange", () => {
  it("offers the longest range Tempo searches", () => {
    expect(
      shorterRange(
        "query limit exceeded: Tempo searches at most 7 days at a time; pick a shorter time range"
      )
    ).toEqual({ from: "now-7d", label: "Last 7 days" })
    expect(
      shorterRange("Tempo searches at most 36 hours at a time; pick one")
    ).toEqual({ from: "now-36h", label: "Last 36 hours" })
  })

  it("names limits under a day in hours or minutes", () => {
    expect(
      shorterRange("Tempo searches at most 90 minutes at a time; pick one")
    ).toEqual({ from: "now-90m", label: "Last 90 minutes" })
    expect(
      shorterRange("Tempo searches at most 30 minutes at a time; pick one")
    ).toEqual({ from: "now-30m", label: "Last 30 minutes" })
    expect(
      shorterRange("Tempo searches at most 1 hour at a time; pick one")
    ).toEqual({ from: "now-1h", label: "Last 1 hour" })
    expect(
      shorterRange("Tempo searches at most 1 day at a time; pick one")
    ).toEqual({ from: "now-1d", label: "Last 1 day" })
  })

  it("leaves other failures alone", () => {
    expect(shorterRange("query timed out")).toBeNull()
  })
})
