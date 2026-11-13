import { describe, expect, it } from "vitest"

import { accountChanges } from "./account-changes"

const account = { role: "editor", takes_runs: false } as const

describe("accountChanges", () => {
  it.each([
    [{ role: "editor", takesRuns: false }, null],
    [{ role: "viewer", takesRuns: false }, { role: "viewer" }],
    [{ role: "editor", takesRuns: true }, { takes_runs: true }],
    [
      { role: "admin", takesRuns: true },
      { role: "admin", takes_runs: true },
    ],
  ] as const)("sends only what differs for %j", (wanted, expected) => {
    expect(accountChanges(account, wanted)).toEqual(expected)
  })

  it("turns takes_runs off", () => {
    expect(
      accountChanges(
        { role: "editor", takes_runs: true },
        { role: "editor", takesRuns: false }
      )
    ).toEqual({ takes_runs: false })
  })
})
