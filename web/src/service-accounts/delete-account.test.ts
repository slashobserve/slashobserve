import { describe, expect, it } from "vitest"

import { ApiError, problemText } from "@/api/errors"

import {
  deleteAccountProblems,
  deleteAccountText,
  liveKeyCounts,
} from "./delete-account"

describe("liveKeyCounts", () => {
  it("counts only keys that still work, per account", () => {
    const counts = liveKeyCounts([
      { service_account_id: "a", state: "active" },
      { service_account_id: "a", state: "expiring" },
      { service_account_id: "a", state: "revoked" },
      { service_account_id: "b", state: "expired" },
    ])
    expect(counts.get("a")).toBe(2)
    expect(counts.get("b")).toBeUndefined()
  })
})

describe("deleteAccountText", () => {
  it("says how many keys stop", () => {
    expect(deleteAccountText("ci", 0)).toMatch(/^It has no working keys\./)
    expect(deleteAccountText("ci", 1)).toMatch(/^Its 1 working key stops/)
    expect(deleteAccountText("ci", 3)).toMatch(/^Its 3 working keys stop/)
  })
})

describe("deleteAccountProblemText", () => {
  it("explains a file account and a missing one", () => {
    expect(
      problemText(new ApiError(409, "provisioned", "x"), deleteAccountProblems)
    ).toMatch(/service-accounts file/)
    expect(
      problemText(new ApiError(404, "not_found", "x"), deleteAccountProblems)
    ).toBe("This service account no longer exists.")
  })
})
