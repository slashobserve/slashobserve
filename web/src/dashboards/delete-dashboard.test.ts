import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  deleteDescription,
  deleteOutcome,
  neighbourAfterDelete,
} from "./delete-dashboard"

describe("deleteDescription", () => {
  it.each([
    [
      1,
      "This deletes it for everyone, with its 1 version, comments and agent history. This cannot be undone.",
    ],
    [
      3,
      "This deletes it for everyone, with its 3 versions, comments and agent history. This cannot be undone.",
    ],
  ])("words %i versions", (versions, expected) => {
    expect(deleteDescription(versions)).toBe(expected)
  })
})

describe("deleteOutcome", () => {
  it("treats a 404 as already deleted", () => {
    expect(
      deleteOutcome(new ApiError(404, "not_found", "dashboard not found"))
    ).toEqual({ kind: "gone" })
  })

  it("explains a dashboard the agent is still creating", () => {
    expect(
      deleteOutcome(
        new ApiError(409, "dashboard_being_created", "still being created")
      )
    ).toEqual({
      kind: "problem",
      text: "The agent is still creating this dashboard. Try again when it finishes.",
    })
  })

  it("shows the server's message for any other failure", () => {
    expect(
      deleteOutcome(new ApiError(500, "internal", "the database is down"))
    ).toEqual({ kind: "problem", text: "The database is down." })
  })

  it("shows a generic message for an unknown failure", () => {
    expect(deleteOutcome(new Error("boom"))).toEqual({
      kind: "problem",
      text: "Something went wrong. Try again.",
    })
  })
})

describe("neighbourAfterDelete", () => {
  it.each([
    ["in the middle", ["a", "b", "c"], "b", "c"],
    ["at the end", ["a", "b", "c"], "c", "b"],
    ["at the start", ["a", "b", "c"], "a", "b"],
    ["alone", ["a"], "a", null],
    ["absent", ["a", "b"], "z", null],
    ["in an empty list", [], "a", null],
  ])("picks the neighbour when %s", (_name, ids, deleted, expected) => {
    expect(neighbourAfterDelete(ids, deleted)).toBe(expected)
  })
})
