import { describe, expect, it } from "vitest"

import { saveBlockedText } from "./save-blocked"

const none = { deleted: false, jsonBlocked: false, runBlocked: false }

describe("saveBlockedText", () => {
  it("is null when nothing stops the save", () => {
    expect(saveBlockedText(none)).toBeNull()
  })

  it("says to fix the JSON or the query", () => {
    expect(saveBlockedText({ ...none, jsonBlocked: true })).toBe(
      "Fix the JSON to save."
    )
    expect(saveBlockedText({ ...none, runBlocked: true })).toBe(
      "Fix the query to save."
    )
  })

  it("puts a deleted dashboard before any fix, because no fix lets the save through", () => {
    expect(
      saveBlockedText({ deleted: true, jsonBlocked: true, runBlocked: true })
    ).toBe("This dashboard was deleted, so this draft cannot be saved.")
  })
})
