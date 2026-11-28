import { describe, expect, test } from "vitest"

import { ApiError, problemText } from "@/api/errors"

import { DatasourceError } from "./admin-queries"
import { saveProblems, usersSummary } from "./problems"

function datasourceError(status: number, code: string, body: unknown = {}) {
  return new DatasourceError(new ApiError(status, code, "server text"), body)
}

describe("DatasourceError", () => {
  test("keeps the fields and the users the server listed", () => {
    const error = datasourceError(409, "datasource_in_use", {
      used_by: [{ kind: "dashboard", id: "d1", title: "Checkout" }],
      hidden: 2,
    })
    expect(error.usedBy).toHaveLength(1)
    expect(error.hidden).toBe(2)
    expect(error.fields).toEqual([])
  })
})

describe("saveProblemText", () => {
  test("says what to change for the codes the server sends", () => {
    expect(
      problemText(datasourceError(409, "secret_key_missing"), saveProblems)
    ).toMatch(/SLASHOBSERVE_SECRET_KEY/)
    expect(
      problemText(datasourceError(409, "uid_taken"), saveProblems)
    ).toMatch(/id/)
    expect(
      problemText(datasourceError(422, "secret_required"), saveProblems)
    ).toMatch(/credential again/)
    expect(
      problemText(datasourceError(409, "datasource_changed"), saveProblems)
    ).toMatch(/Reload/)
    expect(
      problemText(
        datasourceError(403, "outbound_domain_not_allowed"),
        saveProblems
      )
    ).toMatch(/Outbound access/)
    expect(
      problemText(datasourceError(404, "not_found"), saveProblems)
    ).toMatch(/no longer exists/)
  })

  test("falls back to the server sentence for other errors", () => {
    expect(problemText(datasourceError(500, "internal"), saveProblems)).toBe(
      "Server text."
    )
  })
})

describe("usersSummary", () => {
  test("lists users and counts the ones in hidden folders", () => {
    expect(
      usersSummary(
        [
          { kind: "dashboard", id: "d1", title: "Checkout" },
          { kind: "saved_search", id: "s1", title: "Errors" },
        ],
        1
      ).map((line) => [line.text, line.dashboard?.id])
    ).toEqual([
      ["Dashboard: Checkout", "d1"],
      ["Saved search: Errors", undefined],
      ["1 more in a folder you cannot see", undefined],
    ])
    expect(usersSummary([], 3).map((line) => line.text)).toEqual([
      "3 more in folders you cannot see",
    ])
  })
})
