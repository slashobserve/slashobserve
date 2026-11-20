import { describe, expect, test } from "vitest"

import {
  emptyPromptText,
  isLongPrompt,
  promptCounter,
  promptLength,
} from "./prompt-length"

describe("promptCounter", () => {
  test("stays hidden while there is room", () => {
    expect(promptCounter("a".repeat(3599))).toBeNull()
  })

  test("appears at 90 percent of the limit", () => {
    expect(promptCounter("a".repeat(3600))).toEqual({
      text: "3600 of 4000",
      over: false,
    })
  })

  test("is not over at the limit and is over one past it", () => {
    expect(promptCounter("a".repeat(4000))?.over).toBe(false)
    expect(promptCounter("a".repeat(4001))).toEqual({
      text: "4001 of 4000",
      over: true,
    })
  })

  test("counts characters like the server, not UTF-16 units, and ignores outer space", () => {
    expect(promptLength("  😀😀  ")).toBe(2)
  })
})

describe("emptyPromptText", () => {
  test("names a question or a dashboard for a chat", () => {
    expect(emptyPromptText({})).toBe(
      "Ask a question or describe a dashboard first."
    )
  })

  test("names the change for a dashboard", () => {
    expect(emptyPromptText({ dashboardId: "d1" })).toBe(
      "Describe the change you want first."
    )
  })
})

describe("isLongPrompt", () => {
  test.each([
    ["a short line", "checkout health", false],
    ["600 characters", "a".repeat(600), false],
    ["601 characters", "a".repeat(601), true],
    ["6 lines", Array(6).fill("x").join("\n"), false],
    ["7 lines", Array(7).fill("x").join("\n"), true],
  ])("%s", (_name, text, expected) => {
    expect(isLongPrompt(text)).toBe(expected)
  })
})
