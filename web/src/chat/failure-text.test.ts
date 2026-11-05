import { describe, expect, it } from "vitest"

import { failureText } from "./failure-text"

describe("failureText", () => {
  it("explains a known code and what to do next", () => {
    expect(failureText("rate_limited")).toEqual({
      meaning: "The model provider is limiting requests from this server.",
      next: "Wait a minute, then try again.",
    })
  })

  it("gives codes that share a meaning the same text", () => {
    expect(failureText("lease_expired")).toEqual(failureText("crashed"))
    expect(failureText("check_failed")?.meaning).toBe(
      "The agent could not get its queries past the checks."
    )
  })

  it("explains a run that ended without a finding", () => {
    expect(failureText("no_finding")?.meaning).toBe(
      "The agent ended without writing a finding."
    )
  })

  it.each([null, "internal", "toString", "something_new"])(
    "returns null for %s so the server sentence shows",
    (code) => {
      expect(failureText(code)).toBeNull()
    }
  )
})
