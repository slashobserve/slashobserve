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
    expect(unverifiedReasons(unverified, 3, 3).panels).toEqual(
      new Map([["p2", "first"]])
    )
  })

  it("maps each variable to its first issue's message", () => {
    expect(unverifiedReasons(unverified, 3, 3).variables).toEqual(
      new Map([["env", "variable failed"]])
    )
  })

  it("is empty when the drawn version is not the one that lists them", () => {
    const reasons = unverifiedReasons(unverified, 3, 2)
    expect(reasons.panels.size).toBe(0)
    expect(reasons.variables.size).toBe(0)
  })

  it("is empty when nothing is unverified", () => {
    const reasons = unverifiedReasons({ panels: [], variables: [] }, 1, 1)
    expect(reasons.panels.size).toBe(0)
    expect(reasons.variables.size).toBe(0)
  })
})
