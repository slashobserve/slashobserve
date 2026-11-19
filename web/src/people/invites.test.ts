import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { inviteEmailProblem, inviteProblemText } from "./invites"

describe("inviteEmailProblem", () => {
  it("accepts one address and ignores surrounding space", () => {
    expect(inviteEmailProblem(" new@example.com ")).toBeNull()
  })

  it.each([
    ["", "Enter an email address."],
    ["   ", "Enter an email address."],
    ["new", "Enter one email address, like name@example.com."],
    ["@example.com", "Enter one email address, like name@example.com."],
    ["new@", "Enter one email address, like name@example.com."],
    ["a@b.com c@d.com", "Enter one email address, like name@example.com."],
  ])("refuses %j", (email, text) => {
    expect(inviteEmailProblem(email)).toBe(text)
  })

  it("refuses an address longer than the server takes", () => {
    expect(inviteEmailProblem(`${"a".repeat(250)}@b.co`)).toBe(
      "Email addresses are at most 254 characters."
    )
  })
})

describe("inviteProblemText", () => {
  it("tells the Admin what to change for each refusal", () => {
    expect(inviteProblemText(new ApiError(409, "already_member", "x"))).toMatch(
      /Change their role/
    )
    expect(
      inviteProblemText(new ApiError(422, "email_domain_not_allowed", "x"))
    ).toMatch(/allowed email domains/)
    expect(inviteProblemText(new ApiError(404, "not_found", "x"))).toMatch(
      /no longer exists/
    )
  })
})
