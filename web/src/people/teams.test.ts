import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import type { User } from "./queries"
import {
  candidatesFor,
  teamNameProblem,
  teamProblemText,
  type Team,
} from "./teams"

describe("teamNameProblem", () => {
  it("accepts a name and refuses an empty or long one", () => {
    expect(teamNameProblem(" sre ")).toBeNull()
    expect(teamNameProblem("  ")).toBe("Enter a name.")
    expect(teamNameProblem("x".repeat(101))).toBe(
      "Team names are at most 100 characters."
    )
  })
})

describe("candidatesFor", () => {
  it("offers active people who are not members yet", () => {
    const people = [
      { id: "a", deactivated_at: null },
      { id: "b", deactivated_at: null },
      { id: "c", deactivated_at: "2026-11-19T00:00:00Z" },
    ] as User[]
    const team = { members: [{ id: "a" }] } as Team
    expect(candidatesFor(team, people).map((p) => p.id)).toEqual(["b"])
  })
})

describe("teamProblemText", () => {
  it("says what to change", () => {
    expect(teamProblemText(new ApiError(409, "team_exists", "x"))).toBe(
      "A team with this name already exists."
    )
    expect(teamProblemText(new ApiError(422, "too_many_teams", "x"))).toMatch(
      /Delete one first/
    )
  })
})
