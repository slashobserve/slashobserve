import { describe, expect, it } from "vitest"

import { canSaveAsPrompt, savePromptButtonId } from "./save-prompt-target"

describe("canSaveAsPrompt", () => {
  it("offers only new-dashboard chats", () => {
    expect(canSaveAsPrompt({ source: "observe" })).toBe(true)
    expect(canSaveAsPrompt({ source: "prompt" })).toBe(false)
    expect(canSaveAsPrompt({ source: "comment" })).toBe(false)
  })
})

describe("savePromptButtonId", () => {
  it("names the button after the request", () => {
    expect(savePromptButtonId("abc-123")).toBe("history-save-prompt-abc-123")
  })
})
