import { describe, expect, it } from "vitest"

import type { HealthItem, HealthProblem } from "@/health/health-labels"

import { pinProblemText } from "./pin-problems"

function item(...kinds: HealthProblem["kind"][]): HealthItem {
  return {
    id: "d1",
    problems: kinds.map((kind) => ({ kind })),
  } as unknown as HealthItem
}

describe("pinProblemText", () => {
  it("shows nothing before the health read arrives", () => {
    expect(pinProblemText(undefined)).toBeNull()
  })

  it("shows nothing for a healthy dashboard", () => {
    expect(pinProblemText(item())).toBeNull()
  })

  it("counts one problem and several problems", () => {
    expect(pinProblemText(item("query_failed"))).toBe("1 problem")
    expect(pinProblemText(item("query_failed", "no_data"))).toBe("2 problems")
  })

  it("does not count the unopened note as a problem", () => {
    expect(pinProblemText(item("unopened"))).toBeNull()
    expect(pinProblemText(item("unopened", "no_data"))).toBe("1 problem")
  })
})
