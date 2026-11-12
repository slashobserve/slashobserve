import { describe, expect, it } from "vitest"

import { maxSummaryLength, summaryProblem } from "./summary"

describe("summaryProblem", () => {
  it.each([
    ["", "Write a summary"],
    ["   \n  ", "Write a summary"],
    ["Rolled back the release\nand reopened checkout", "Keep it to one line"],
    ["a".repeat(maxSummaryLength + 1), "Keep it under 500 characters"],
    ["a".repeat(maxSummaryLength), null],
    ["  Rolled back the release  \n", null],
    ["Rolled back the release", null],
  ])("answers %j with %j", (text, expected) => {
    expect(summaryProblem(text)).toBe(expected)
  })

  it("counts characters, not UTF-16 units", () => {
    expect(summaryProblem("🔥".repeat(maxSummaryLength))).toBeNull()
  })
})
