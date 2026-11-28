import { describe, expect, it } from "vitest"

import { ApiError, problemText } from "@/api/errors"

import { actionsFor, lifecycleProblems, removalText } from "./person-lifecycle"
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

  it("offers reactivate and remove for a deactivated person", () => {
    expect(actionsFor(person("b", "2026-11-19T00:00:00Z"), "a")).toEqual([
      "reactivate",
      "remove",
    ])
  })
})

describe("removalText", () => {
  it("says an empty My Dashboards is empty", () => {
    expect(
      removalText({ dashboards: 0, observations: 0, saved_searches: 0 })
    ).toBe("Their My Dashboards is empty.")
  })

  it("names each kind that is there", () => {
    expect(
      removalText({ dashboards: 1, observations: 0, saved_searches: 0 })
    ).toBe("Their My Dashboards holds 1 dashboard, deleted with them.")
    expect(
      removalText({ dashboards: 3, observations: 1, saved_searches: 2 })
    ).toBe(
      "Their My Dashboards holds 3 dashboards, 1 observation and 2 saved searches, deleted with them."
    )
  })
})

describe("lifecycleProblemText", () => {
  it("says a missing person is gone", () => {
    expect(
      problemText(
        new ApiError(404, "not_found", "user not found"),
        lifecycleProblems
      )
    ).toBe("This person no longer exists.")
  })

  it("says to deactivate again when someone reactivated the person", () => {
    expect(
      problemText(new ApiError(409, "not_deactivated", "x"), lifecycleProblems)
    ).toMatch(/Deactivate them before removing them/)
  })

  it("passes on what the server says to change", () => {
    expect(
      problemText(
        new ApiError(
          409,
          "last_admin",
          "this is the last active Admin; make someone else an Admin first"
        ),
        lifecycleProblems
      )
    ).toBe("This is the last active Admin; make someone else an Admin first.")
  })
})
