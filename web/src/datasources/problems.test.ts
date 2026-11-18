import { describe, expect, test } from "vitest"

import { ApiError } from "@/api/errors"

import { DatasourceError } from "./admin-queries"
import { saveProblemText, usersSummary } from "./problems"

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
    expect(saveProblemText(datasourceError(409, "secret_key_missing"))).toMatch(
      /SLASHOBSERVE_SECRET_KEY/
    )
    expect(saveProblemText(datasourceError(409, "uid_taken"))).toMatch(/id/)
    expect(
      saveProblemText(datasourceError(403, "outbound_domain_not_allowed"))
    ).toMatch(/Outbound access/)
    expect(saveProblemText(datasourceError(404, "not_found"))).toMatch(
      /no longer exists/
    )
  })

  test("falls back to the server sentence for other errors", () => {
    expect(saveProblemText(datasourceError(500, "internal"))).toBe(
      "Server text."
    )
  })
})

describe("usersSummary", () => {
  test("lists users and counts the ones in hidden folders", () => {
    expect(
      usersSummary(
        [
          { kind: "dashboard", title: "Checkout" },
          { kind: "saved_search", title: "Errors" },
        ],
        1
      )
    ).toEqual([
      "Dashboard: Checkout",
      "Saved search: Errors",
      "1 more in a folder you cannot see",
    ])
    expect(usersSummary([], 3)).toEqual(["3 more in folders you cannot see"])
  })
})
