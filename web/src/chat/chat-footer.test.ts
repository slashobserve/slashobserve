import { describe, expect, test } from "vitest"

import { chatFooterText } from "./chat-footer"

describe("chatFooterText", () => {
  test.each([
    ["no summary", null, false, null],
    ["no summary with more pages", null, true, null],
    ["no matching chats", { total: 0, people: null }, false, null],
    ["no matching chats on everyone", { total: 0, people: 0 }, true, null],
    ["one chat", { total: 1, people: null }, false, "1 chat."],
    [
      "six chats with more",
      { total: 6, people: null },
      true,
      "6 chats. Older chats load as you scroll.",
    ],
    ["a thousand chats", { total: 1234, people: null }, false, "1,234 chats."],
    [
      "one chat from one person",
      { total: 1, people: 1 },
      false,
      "1 chat from 1 person.",
    ],
    [
      "nine chats from four people with more",
      { total: 9, people: 4 },
      true,
      "9 chats from 4 people. Older chats load as you scroll.",
    ],
  ] as const)("says %s", (_name, summary, more, text) => {
    expect(chatFooterText(summary, more)).toBe(text)
  })
})
