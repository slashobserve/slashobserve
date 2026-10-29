import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import { promptProblem, type PromptProblem } from "./queries"

const mine = { name: "Checkout health", shared: false }
const shared = { name: "Checkout health", shared: true }

const cases: {
  name: string
  error: unknown
  attempt: { name: string; shared: boolean }
  expected: PromptProblem
}[] = [
  {
    name: "a network failure",
    error: new NetworkError("x"),
    attempt: mine,
    expected: { field: null, text: "Could not reach the server. Try again." },
  },
  {
    name: "a taken private name",
    error: new ApiError(409, "name_taken", "taken"),
    attempt: mine,
    expected: {
      field: "name",
      text: 'You already have a prompt named "Checkout health".',
    },
  },
  {
    name: "a taken shared name",
    error: new ApiError(409, "name_taken", "taken"),
    attempt: shared,
    expected: {
      field: "name",
      text: 'Another shared prompt is named "Checkout health". Pick another name, or keep it to yourself.',
    },
  },
  {
    name: "an invalid name",
    error: new ApiError(422, "invalid_name", "bad"),
    attempt: mine,
    expected: {
      field: "name",
      text: "Use letters, numbers, spaces, - and _, starting and ending with a letter or number.",
    },
  },
  {
    name: "an invalid body",
    error: new ApiError(422, "invalid_body", "bad"),
    attempt: mine,
    expected: {
      field: "body",
      text: "Use 1 to 4000 characters, without control characters.",
    },
  },
  {
    name: "too many prompts",
    error: new ApiError(422, "too_many_prompts", "many"),
    attempt: mine,
    expected: {
      field: null,
      text: "You have 100 saved prompts, the most you can keep. Delete one first.",
    },
  },
  {
    name: "a missing prompt",
    error: new ApiError(404, "not_found", "missing"),
    attempt: mine,
    expected: { field: null, text: "This prompt no longer exists." },
  },
  {
    name: "a prompt someone else owns",
    error: new ApiError(403, "not_owner", "no"),
    attempt: mine,
    expected: {
      field: null,
      text: "Only the owner or an admin can change this prompt.",
    },
  },
  {
    name: "a role that cannot save",
    error: new ApiError(403, "forbidden", "no"),
    attempt: mine,
    expected: { field: null, text: "Your role cannot save prompts." },
  },
  {
    name: "another API error",
    error: new ApiError(500, "internal", "something broke"),
    attempt: mine,
    expected: { field: null, text: "Something broke." },
  },
  {
    name: "an unknown failure",
    error: new Error("boom"),
    attempt: mine,
    expected: { field: null, text: "Something went wrong. Try again." },
  },
]

describe("promptProblem", () => {
  it.each(cases)("maps $name", ({ error, attempt, expected }) => {
    expect(promptProblem(error, attempt)).toEqual(expected)
  })
})
