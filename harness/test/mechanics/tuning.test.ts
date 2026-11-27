import { describe, expect, test } from "bun:test"
import {
  cutOffEarly,
  forCall,
  type Tuning,
} from "../../src/session/mechanics/tuning"

describe("forCall", () => {
  const cases: Array<[string, Tuning, boolean, Tuning]> = [
    [
      "planning steps low up to medium",
      { effort: "low", maxTokens: 16000 },
      true,
      { effort: "medium", maxTokens: 16000 },
    ],
    [
      "planning steps medium up to high",
      { effort: "medium", maxTokens: 16000 },
      true,
      { effort: "high", maxTokens: 16000 },
    ],
    [
      "planning leaves high alone",
      { effort: "high", maxTokens: 16000 },
      true,
      { effort: "high", maxTokens: 16000 },
    ],
    [
      "planning leaves xhigh alone",
      { effort: "xhigh", maxTokens: 16000 },
      true,
      { effort: "xhigh", maxTokens: 16000 },
    ],
    [
      "planning leaves none alone",
      { effort: "none", maxTokens: 16000 },
      true,
      { effort: "none", maxTokens: 16000 },
    ],
    [
      "planning leaves an empty effort empty",
      { effort: "", maxTokens: 16000 },
      true,
      { effort: "", maxTokens: 16000 },
    ],
    [
      "a save call keeps the effort and halves 16000 to the floor",
      { effort: "medium", maxTokens: 16000 },
      false,
      { effort: "medium", maxTokens: 8192 },
    ],
    [
      "a save call halves 32000",
      { effort: "low", maxTokens: 32000 },
      false,
      { effort: "low", maxTokens: 16000 },
    ],
    [
      "a save call never goes above the configured 4096",
      { effort: "", maxTokens: 4096 },
      false,
      { effort: "", maxTokens: 4096 },
    ],
    [
      "a save call never goes above the configured 1024",
      { effort: "", maxTokens: 1024 },
      false,
      { effort: "", maxTokens: 1024 },
    ],
    [
      "a save call at 64000",
      { effort: "", maxTokens: 64000 },
      false,
      { effort: "", maxTokens: 32000 },
    ],
  ]
  test.each(cases)("%s", (_name, tuning, planning, want) => {
    expect(forCall(tuning, planning)).toEqual(want)
  })
})

describe("cutOffEarly", () => {
  const configured: Tuning = { effort: "", maxTokens: 16000 }
  const cases: Array<[string, number, "max_tokens" | "tool_use", boolean]> = [
    ["cut at the lowered cap", 8192, "max_tokens", true],
    ["cut at the configured cap", 16000, "max_tokens", false],
    ["finished under the lowered cap", 8192, "tool_use", false],
  ]
  test.each(cases)("%s", (_name, maxTokens, stop, want) => {
    expect(cutOffEarly(configured, { effort: "", maxTokens }, stop)).toBe(want)
  })
})
