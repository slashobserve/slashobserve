import { describe, expect, test } from "vitest"

import { ApiError } from "@/api/errors"

import { noteProblem, outcomeOf } from "./request-access"

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
