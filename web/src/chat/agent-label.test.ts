import { describe, expect, it } from "vitest"

import { agentLabel } from "./agent-label"

describe("agentLabel", () => {
  it.each([
    ["no agent info", undefined, null],
    ["no model", { model: null, effort: "high" }, null],
    ["a model without effort", { model: "gpt-5", effort: null }, "gpt-5"],
    [
      "a model with effort",
      { model: "claude-haiku-5-5", effort: "medium" },
      "claude-haiku-5-5, medium effort",
    ],
  ])("labels %s", (_name, agent, expected) => {
    expect(agentLabel(agent)).toBe(expected)
  })
})
