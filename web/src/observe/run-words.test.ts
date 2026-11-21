import { describe, expect, it } from "vitest"

import { runWords } from "./run-words"

describe("runWords", () => {
  it("gives the prompt of a run the reader may read", () => {
    expect(runWords({ prompt: "Why is p99 up?", redacted: false })).toBe(
      "Why is p99 up?"
    )
  })

  it("gives nothing for a run from someone's private chat", () => {
    expect(runWords({ prompt: "", redacted: true })).toBeNull()
  })
})
