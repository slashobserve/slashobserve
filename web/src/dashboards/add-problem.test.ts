import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { addProblem } from "./add-problem"

describe("addProblem", () => {
  it("keeps the stale-version text for a version conflict only", () => {
    const problem = addProblem(
      new ApiError(409, "version_conflict", "parent version is not the latest")
    )
    expect(problem.lines).toEqual([
      "Someone saved this dashboard a moment ago. Add again to place the panel on the latest version.",
    ])
  })

  it.each(["restricted_content", "admins_only_folder"])(
    "shows the server message for a %s refusal",
    (code) => {
      const message = "this reads the Audit trail, which only Admins can query"
      expect(addProblem(new ApiError(409, code, message))).toEqual({
        lines: [message],
      })
    }
  )

  it("lists the issues of a refused query with the hint", () => {
    const problem = addProblem(
      new ApiError(422, "invalid_spec", "invalid", [
        { path: "/panels/0", code: "query_invalid", message: "bad query" },
      ])
    )
    expect(problem.lines).toHaveLength(1)
    expect(problem.hint).toContain("dashboard's own time range")
  })

  it("says the dashboard cannot be edited on a 403", () => {
    expect(addProblem(new ApiError(403, "forbidden", "no")).lines).toEqual([
      "You cannot edit this dashboard.",
    ])
  })
})
