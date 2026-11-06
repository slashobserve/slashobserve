import { describe, expect, it } from "vitest"

import {
  freshCursor,
  parseStoredHistory,
  promptHistoryLimit,
  pushPrompt,
  stepHistory,
} from "./prompt-history"

describe("parseStoredHistory", () => {
  it.each([
    ["nothing stored", null],
    ["malformed JSON", "[not json"],
    ["an object", '{"a":"b"}'],
    ["a mixed array", '["ok", 3]'],
  ])("returns an empty list for %s", (_name, raw) => {
    expect(parseStoredHistory(raw)).toEqual([])
  })

  it("keeps a stored list of strings", () => {
    expect(parseStoredHistory('["newer","older"]')).toEqual(["newer", "older"])
  })
})

describe("pushPrompt", () => {
  it("puts the trimmed prompt first", () => {
    expect(pushPrompt(["older"], "  newer  ")).toEqual(["newer", "older"])
  })

  it("moves an equal older entry to the front", () => {
    expect(pushPrompt(["a", "b", "c"], "c")).toEqual(["c", "a", "b"])
  })

  it("ignores a blank prompt", () => {
    expect(pushPrompt(["a"], "   ")).toEqual(["a"])
  })

  it("keeps at most 20 prompts", () => {
    const full = Array.from({ length: promptHistoryLimit }, (_, i) => `p${i}`)
    const next = pushPrompt(full, "new")
    expect(next).toHaveLength(20)
    expect(next[0]).toBe("new")
    expect(next).not.toContain("p19")
  })
})

describe("stepHistory", () => {
  const history = ["newest", "middle", "oldest"]

  it("saves the draft and shows the newest entry going older", () => {
    expect(stepHistory(history, freshCursor, "older", "typing")).toEqual({
      cursor: { index: 0, draft: "typing" },
      text: "newest",
    })
  })

  it("steps older and stops past the last entry", () => {
    const second = stepHistory(
      history,
      { index: 0, draft: "typing" },
      "older",
      "newest"
    )
    expect(second).toEqual({
      cursor: { index: 1, draft: "typing" },
      text: "middle",
    })
    expect(
      stepHistory(history, { index: 2, draft: "typing" }, "older", "oldest")
    ).toBeNull()
  })

  it("returns to the saved draft going newer from the newest entry", () => {
    expect(
      stepHistory(history, { index: 0, draft: "typing" }, "newer", "newest")
    ).toEqual({ cursor: { index: -1, draft: "typing" }, text: "typing" })
  })

  it("does nothing going newer while not browsing", () => {
    expect(stepHistory(history, freshCursor, "newer", "typing")).toBeNull()
  })

  it("does nothing going older with no history", () => {
    expect(stepHistory([], freshCursor, "older", "")).toBeNull()
  })
})
