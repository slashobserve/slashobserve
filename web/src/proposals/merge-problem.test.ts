import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import { mergeLeftProposalStale, mergeProblem } from "./merge-problem"

describe("mergeProblem", () => {
  it.each([
    [
      "proposal_changed",
      409,
      "Changed.",
      "This proposal has a newer revision. Review it and merge again.",
    ],
    [
      "version_conflict",
      409,
      "Changed.",
      "The dashboard changed while merging. Review the conflicts and merge again.",
    ],
    ["invalid_transition", 409, "Closed.", "This proposal is already closed."],
    [
      "propose_mode",
      403,
      "this connection proposes changes",
      "This connection proposes changes.",
    ],
    ["forbidden_role", 403, "you need edit access", "You need edit access."],
    ["invalid_spec", 422, "Panel p1 fails.", "Panel p1 fails."],
  ])("explains %s", (code, status, message, expected) => {
    expect(mergeProblem(new ApiError(status, code, message))).toBe(expected)
  })

  it("says the server is unreachable on a network failure", () => {
    expect(mergeProblem(new NetworkError("offline"))).toMatch(/reach|connect/i)
  })

  it("gives a plain sentence for an unknown failure", () => {
    expect(mergeProblem(new Error("boom"))).toBe(
      "Something went wrong. Try again."
    )
  })
})

describe("mergeLeftProposalStale", () => {
  it.each([
    ["proposal_changed", true],
    ["version_conflict", true],
    ["invalid_transition", true],
    ["propose_mode", false],
    ["invalid_spec", false],
  ])("for %s is %s", (code, expected) => {
    expect(mergeLeftProposalStale(new ApiError(409, code, "x"))).toBe(expected)
  })

  it("is false for a failure that is not an API error", () => {
    expect(mergeLeftProposalStale(new Error("boom"))).toBe(false)
  })
})
