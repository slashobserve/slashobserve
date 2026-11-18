import { describe, expect, it } from "vitest"

import { maxPromptLength } from "@/prompts/rules"

import { missingPanelsPrompt, plannedPanels } from "./result-card"

describe("plannedPanels", () => {
  it("reads the plan of a build saved before it finished", () => {
    expect(
      plannedPanels(
        "Saved what the agent built before it stopped: 5 of 8 panels"
      )
    ).toBe(8)
  })

  it.each([
    "Create dashboard",
    "Saved what the agent built before it stopped: 1 panel",
    undefined,
  ])("is null for %s", (summary) => {
    expect(plannedPanels(summary)).toBeNull()
  })
})

describe("missingPanelsPrompt", () => {
  it("names the original request", () => {
    expect(missingPanelsPrompt("checkout health")).toBe(
      "Add the panels that are still missing from this dashboard. The request was: checkout health"
    )
  })

  it("stays within the prompt limit", () => {
    expect(
      [...missingPanelsPrompt("x".repeat(maxPromptLength))].length
    ).toBeLessThanOrEqual(maxPromptLength)
  })
})
