import { describe, expect, it } from "vitest"

import { keptTitleNote } from "./shared-title"

describe("keptTitleNote", () => {
  it("says which title was left as it is", () => {
    expect(keptTitleNote("chat", "kept")).toBe(
      "The observation made from this chat keeps its title, because you cannot edit it."
    )
    expect(keptTitleNote("observation", "kept")).toBe(
      "The chat this observation was made from keeps its title, because only the person who started it can rename it."
    )
  })

  it("says nothing when both titles changed or nothing is shared", () => {
    expect(keptTitleNote("chat", "renamed")).toBeNull()
    expect(keptTitleNote("observation", undefined)).toBeNull()
  })
})
