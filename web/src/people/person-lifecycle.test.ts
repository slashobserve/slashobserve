import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { actionsFor, lifecycleProblemText } from "./person-lifecycle"
import type { User } from "./queries"

function person(id: string, deactivated: string | null): User {
  return { id, deactivated_at: deactivated } as User
}

describe("actionsFor", () => {
  it("offers deactivate for someone else who is active", () => {
    expect(actionsFor(person("b", null), "a")).toEqual(["deactivate"])
  })

  it("offers nothing for yourself", () => {
    expect(actionsFor(person("a", null), "a")).toEqual([])
  })

  it("offers reactivate for a deactivated person", () => {
    expect(actionsFor(person("b", "2026-11-19T00:00:00Z"), "a")).toEqual([
      "reactivate",
    ])
  })
})

describe("lifecycleProblemText", () => {
  it("says a missing person is gone", () => {
    expect(
      lifecycleProblemText(new ApiError(404, "not_found", "user not found"))
    ).toBe("This person no longer exists.")
  })

  it("passes on what the server says to change", () => {
    expect(
      lifecycleProblemText(
        new ApiError(
          409,
          "last_admin",
          "this is the last active Admin; make someone else an Admin first"
        )
      )
    ).toBe("This is the last active Admin; make someone else an Admin first.")
  })
})
