import { describe, expect, test } from "vitest"

import {
  emptyPromptText,
  isLongPrompt,
  promptCounter,
  promptLength,
} from "./prompt-length"

describe("promptCounter", () => {
  test("stays hidden while more than 500 characters are left", () => {
    expect(promptCounter("a".repeat(3499))).toBeNull()
  })

  test("says how many characters are left from 3500", () => {
    expect(promptCounter("a".repeat(3500))).toEqual({
      text: "500 characters left",
      over: false,
    })
    expect(promptCounter("a".repeat(3999))?.text).toBe("1 character left")
  })

  test("is not over at the limit, counting the trimmed text", () => {
    expect(promptCounter(`  ${"a".repeat(4000)}  `)).toEqual({
      text: "0 characters left",
      over: false,
    })
  })

  test("says how far over the limit the text is", () => {
    expect(promptCounter("a".repeat(4001))).toEqual({
      text: "1 character over",
      over: true,
    })
    expect(promptCounter("a".repeat(4010))?.text).toBe("10 characters over")
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
