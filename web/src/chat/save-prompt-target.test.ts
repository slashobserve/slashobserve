import { describe, expect, it } from "vitest"

import { canSaveAsPrompt, savePromptButtonId } from "./save-prompt-target"

describe("canSaveAsPrompt", () => {
  it("offers only new-dashboard chats", () => {
    const chat = { intent: "apply" as const, comment_id: null }
    expect(canSaveAsPrompt({ ...chat, base_version: null })).toBe(true)
    expect(
      canSaveAsPrompt({ ...chat, intent: "ask", base_version: null })
    ).toBe(true)
    expect(canSaveAsPrompt({ ...chat, base_version: 1 })).toBe(false)
    expect(
      canSaveAsPrompt({ ...chat, base_version: 1, comment_id: "c1" })
    ).toBe(false)
  })
})

describe("savePromptButtonId", () => {
  it("names the button after the request", () => {
    expect(savePromptButtonId("abc-123")).toBe("history-save-prompt-abc-123")
  })
})
