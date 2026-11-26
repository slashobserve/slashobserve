import { describe, expect, it } from "vitest"

import { levelFilterText, openingView, resultsDisplay } from "./logs-results"

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

describe("levelFilterText", () => {
  it("names the levels that stay, in the order of the chips", () => {
    expect(levelFilterText(new Set(["info", "error"]))).toBe(
      "Showing only Warning, Other lines"
    )
  })

  it("names a single level", () => {
    expect(levelFilterText(new Set(["warning", "info", "other"]))).toBe(
      "Showing only Error lines"
    )
  })
})
