import { describe, expect, test } from "vitest"

import { ApiError } from "@/api/errors"

import {
  AuditQueryError,
  auditError,
  markedQuery,
  markedSpan,
  queryProblemOf,
  rejectionOf,
} from "./query-problem"

function response(status: number): Response {
  return new Response(null, { status })
}

describe("markedSpan", () => {
  test("keeps ASCII byte offsets as indexes", () => {
    expect(markedSpan("outcome:)", { start: 8, end: 9 })).toEqual({
      from: 8,
      to: 9,
    })
  })

  test("counts a two-byte letter once", () => {
    const text = 'actor_name:"Zoë" and )'
    expect(markedSpan(text, { start: 22, end: 23 })).toEqual({
      from: 21,
      to: 22,
    })
    expect(text.slice(21, 22)).toBe(")")
  })

  test("counts an emoji as one four-byte character over two indexes", () => {
    const text = "😀 x:)"
    expect(markedSpan(text, { start: 7, end: 8 })).toEqual({ from: 5, to: 6 })
    expect(markedSpan(text, { start: 0, end: 4 })).toEqual({ from: 0, to: 2 })
  })

  test("widens an empty span to one character", () => {
    expect(markedSpan("a:", { start: 1, end: 1 })).toEqual({ from: 1, to: 2 })
    expect(markedSpan("a:", { start: 2, end: 2 })).toEqual({ from: 1, to: 2 })
  })

  test("clamps offsets past the end", () => {
    expect(markedSpan("ab", { start: 5, end: 9 })).toEqual({ from: 1, to: 2 })
  })
})

describe("markedQuery", () => {
  test("splits the text around the mark", () => {
    expect(markedQuery("é and )", { start: 7, end: 8 })).toEqual({
      before: "é and ",
      marked: ")",
      after: "",
    })
  })
})

describe("auditError", () => {
  test("keeps the byte offsets of a 422", () => {
    const error = auditError(response(422), {
      error: 'unexpected ")"',
      code: "query_invalid",
      start: 3,
      end: 4,
    })
    expect(error).toBeInstanceOf(AuditQueryError)
    expect((error as AuditQueryError).span).toEqual({ start: 3, end: 4 })
    expect(error.message).toBe('unexpected ")"')
  })

  test("has no span for a timeout or another status", () => {
    const timeout = auditError(response(422), {
      error: "too slow",
      code: "query_timeout",
    })
    expect((timeout as AuditQueryError).span).toBe(null)
    expect(
      auditError(response(403), { error: "no", code: "forbidden_role" })
    ).not.toBeInstanceOf(AuditQueryError)
  })
})

describe("queryProblemOf", () => {
  test("marks the text only for an invalid query", () => {
    const invalid = new AuditQueryError(
      new ApiError(422, "query_invalid", "expected a value"),
      { start: 6, end: 6 }
    )
    expect(queryProblemOf(invalid, "actor:")).toEqual({
      message: "expected a value",
      marked: { before: "actor", marked: ":", after: "" },
    })
    expect(
      queryProblemOf(new ApiError(422, "query_timeout", "slow"), "a")
    ).toBe(null)
  })
})

describe("rejectionOf", () => {
  test("reports any refused query, marked only when the span is known", () => {
    const slow = new AuditQueryError(
      new ApiError(422, "query_too_expensive", "too slow"),
      null
    )
    expect(rejectionOf(slow, "a")).toEqual({
      message: "too slow",
      marked: null,
    })
    expect(rejectionOf(new ApiError(500, "internal", "boom"), "a")).toBe(null)
    expect(rejectionOf(null, "a")).toBe(null)
  })
})
