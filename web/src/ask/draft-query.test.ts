import { describe, expect, it } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"
import { offlineText } from "@/logs/explore"

import { draftFailure } from "./draft-query"

describe("draftFailure", () => {
  it("reads the reason of a question that cannot be translated", () => {
    expect(
      draftFailure(
        new ApiError(422, "cannot_translate", "The logs have no user field.")
      )
    ).toEqual({
      kind: "cannot_translate",
      reason: "The logs have no user field.",
    })
  })

  it("shows the message of another API error", () => {
    expect(
      draftFailure(new ApiError(503, "agent_unavailable", "No model is set."))
    ).toEqual({ kind: "error", text: "No model is set." })
  })

  it("says the browser is offline for a network error", () => {
    expect(draftFailure(new NetworkError("Failed to fetch"))).toEqual({
      kind: "error",
      text: offlineText,
    })
  })

  it("treats an abort as a cancel", () => {
    expect(
      draftFailure(new DOMException("The user aborted.", "AbortError"))
    ).toEqual({ kind: "aborted" })
  })

  it("falls back to a generic sentence for anything else", () => {
    expect(draftFailure(new Error("boom"))).toEqual({
      kind: "error",
      text: "The question could not be sent. Try again.",
    })
  })
})
