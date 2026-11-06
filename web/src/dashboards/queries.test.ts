import { describe, expect, it } from "vitest"

import { asDashboardSpec } from "./queries"

const invalid = "Server returned an invalid Spec"

describe("asDashboardSpec", () => {
  it("returns a Spec with the dashboard shape as the same reference", () => {
    const spec = { specVersion: "0", title: "Checkout", panels: [] }
    expect(asDashboardSpec(spec)).toBe(spec)
  })

  it.each([
    ["null", null],
    ["a string", "dashboard"],
    [
      "another spec version",
      { specVersion: "1", title: "Checkout", panels: [] },
    ],
    ["missing panels", { specVersion: "0", title: "Checkout" }],
    ["non-array panels", { specVersion: "0", title: "Checkout", panels: {} }],
  ])("throws for %s", (_, raw) => {
    expect(() => asDashboardSpec(raw)).toThrow(invalid)
  })
})
