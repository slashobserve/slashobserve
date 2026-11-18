import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"
import type { LintResult } from "@/query-editor/server-lint"

import { failureHint, syntaxProblem, truncationNote } from "./problems"

function lintOf(query: string, from: number, message = "bad"): LintResult {
  return {
    status: "ok",
    query,
    items: [{ severity: "error", message, from }],
  }
}

describe("syntaxProblem", () => {
  it("gives line, column and a caret under the position", () => {
    const text = "sum(up)\nrate(x[5m]"
    expect(syntaxProblem(lintOf(text, 15), text)).toEqual({
      message: "bad",
      line: 2,
      column: 8,
      source: "rate(x[5m]",
      caret: "       ^",
    })
  })

  it("says nothing for a lint of an older text", () => {
    expect(syntaxProblem(lintOf("up", 0), "up{")).toBeNull()
  })

  it("says nothing when lint is unavailable or has only warnings", () => {
    expect(
      syntaxProblem({ status: "unavailable", query: "up", message: "x" }, "up")
    ).toBeNull()
    expect(
      syntaxProblem(
        {
          status: "ok",
          query: "up",
          items: [{ severity: "warning", message: "w", from: 0 }],
        },
        "up"
      )
    ).toBeNull()
    expect(syntaxProblem(null, "up")).toBeNull()
  })
})

describe("failureHint", () => {
  it("names the limit and how to narrow the query", () => {
    const hint = failureHint(new ApiError(422, "limit_exceeded", "too many"))
    expect(hint).toContain("200 series")
  })

  it("explains a timeout", () => {
    expect(failureHint(new ApiError(504, "query_timeout", "slow"))).toContain(
      "too long"
    )
  })

  it("has no hint for other errors", () => {
    expect(failureHint(new ApiError(400, "bad_query", "x"))).toBeNull()
    expect(failureHint(new Error("x"))).toBeNull()
  })
})

describe("truncationNote", () => {
  it("shows only when the server cut the rows", () => {
    expect(truncationNote(false)).toBeNull()
    expect(truncationNote(true)).toContain("row limit")
  })
})
