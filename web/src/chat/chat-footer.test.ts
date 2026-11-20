import { describe, expect, test } from "vitest"

import { chatFooterText } from "./chat-footer"

describe("chatFooterText", () => {
  test.each([
    ["no chats", 0, null, false, null],
    ["no chats on everyone", 0, 0, true, null],
    ["one chat", 1, null, false, "1 chat."],
    [
      "six chats with more",
      6,
      null,
      true,
      "6 chats so far. Older chats load as you scroll.",
    ],
    ["a thousand chats", 1234, null, false, "1,234 chats."],
    ["one chat from one person", 1, 1, false, "1 chat from 1 person."],
    [
      "nine chats from four people with more",
      9,
      4,
      true,
      "9 chats from 4 people so far. Older chats load as you scroll.",
    ],
  ] as const)("says %s", (_name, chats, people, more, text) => {
    expect(chatFooterText(chats, people, more)).toBe(text)
  })
})
