import { describe, expect, test } from "vitest"

import { threadParam } from "./thread-param"

describe("threadParam", () => {
  test.each([
    ["a thread id", "0a1b2c3d-4e5f", "0a1b2c3d-4e5f"],
    ["the first of a repeated param", ["one", "two"], "one"],
    ["an empty string", "", undefined],
    ["text with spaces", "a b", undefined],
    ["text over 64 characters", "a".repeat(65), undefined],
    ["a number", 7, undefined],
    ["nothing", undefined, undefined],
  ])("%s", (_name, raw, expected) => {
    expect(threadParam(raw)).toBe(expected)
  })
})
