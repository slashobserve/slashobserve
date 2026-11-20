import { describe, expect, test } from "vitest"

import { ApiError, toApiError } from "@/api/errors"

import {
  noteProblem,
  outcomeOf,
  outcomeText,
  requestedAtOf,
} from "./request-access"

describe("noteProblem", () => {
  test.each([
    ["", null],
    ["on call this week", null],
    ["x".repeat(200), null],
    ["  " + "x".repeat(200) + "  ", null],
    ["x".repeat(201), "Keep the note to 200 characters."],
    ["two\nlines", "Keep the note to one line."],
  ])("%j gives %j", (note, problem) => {
    expect(noteProblem(note)).toBe(problem)
  })
})

describe("outcomeOf", () => {
  test.each([
    [new ApiError(409, "request_open", "open"), "open"],
    [new ApiError(409, "already_can_comment", "can"), "can_comment"],
    [new ApiError(404, "not_found", "not found"), "failed"],
    [new Error("offline"), "failed"],
  ])("%o is %s", (error, outcome) => {
    expect(outcomeOf(error)).toBe(outcome)
  })
})

describe("outcomeText", () => {
  const now = Date.parse("2026-11-20T12:00:00Z")

  test("an open request says how long ago it was made", () => {
    expect(outcomeText("open", "2026-11-20T09:00:00Z", now)).toBe(
      "You asked for comment access here 3 h ago, and the request is still open."
    )
    expect(outcomeText("open", "2026-11-19T09:00:00Z", now)).toBe(
      "You asked for comment access here yesterday, and the request is still open."
    )
  })

  test("an open request with no time still says it is open", () => {
    expect(outcomeText("open")).toBe(
      "You already asked for comment access here, and the request is still open."
    )
  })
})

describe("requestedAtOf", () => {
  test("reads the time the server sent with request_open", () => {
    const error = toApiError(new Response(null, { status: 409 }), {
      error: "open",
      code: "request_open",
      requested_at: "2026-11-20T09:00:00Z",
    })
    expect(requestedAtOf(error)).toBe("2026-11-20T09:00:00Z")
    expect(
      requestedAtOf(new ApiError(409, "request_open", "open"))
    ).toBeUndefined()
    expect(requestedAtOf(new Error("offline"))).toBeUndefined()
  })
})
