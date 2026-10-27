import { describe, expect, it } from "vitest"

import { unverifiedReasons } from "./unverified"

const issue = (message: string) => ({
  path: "/panels/1/queries",
  code: "query_empty",
  message,
})

const unverified = {
  panels: [
    { panel_id: "p2", issues: [issue("first"), issue("second")] },
    { panel_id: "p3", issues: [] },
  ],
  variables: [{ name: "env", issues: [issue("variable failed")] }],
}

describe("unverifiedReasons", () => {
  it("maps each panel to its first issue's message", () => {
    expect(unverifiedReasons(unverified, 3, 3)).toEqual(
      new Map([["p2", "first"]])
    )
  })

  it("is empty when the drawn version is not the one that lists them", () => {
    expect(unverifiedReasons(unverified, 3, 2).size).toBe(0)
  })

  it("is empty when no panel is unverified", () => {
    expect(unverifiedReasons({ panels: [], variables: [] }, 1, 1).size).toBe(0)
  })
})
