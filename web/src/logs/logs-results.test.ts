import { describe, expect, it } from "vitest"

import { hiddenLevelsText, openingView, resultsDisplay } from "./logs-results"

describe("openingView", () => {
  it("wraps the lines on a phone so the message is readable", () => {
    expect(openingView(true).wrap).toBe(true)
  })

  it("keeps one line per row on a wide screen", () => {
    expect(openingView(false).wrap).toBe(false)
  })
})

describe("resultsDisplay", () => {
  it("keeps stream labels and structured metadata out of the message column", () => {
    expect(resultsDisplay.showLabels).toBe(false)
  })
})

describe("hiddenLevelsText", () => {
  it("names the hidden levels in the order of the chips", () => {
    expect(hiddenLevelsText(new Set(["info", "error"]))).toBe(
      "Hiding Error, Info lines"
    )
  })
})
