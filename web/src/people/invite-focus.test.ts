import { describe, expect, it } from "vitest"

import { neighbourInviteId } from "./invite-focus"

describe("neighbourInviteId", () => {
  it("picks the next invite for a middle row", () => {
    expect(neighbourInviteId(["a", "b", "c"], "b")).toBe("c")
  })

  it("picks the next invite for the first row", () => {
    expect(neighbourInviteId(["a", "b"], "a")).toBe("b")
  })

  it("picks the previous invite for the last row", () => {
    expect(neighbourInviteId(["a", "b", "c"], "c")).toBe("b")
  })

  it("picks nothing when the invite is the only one", () => {
    expect(neighbourInviteId(["a"], "a")).toBeNull()
  })

  it("picks nothing when the invite is not listed", () => {
    expect(neighbourInviteId(["a", "b"], "z")).toBeNull()
  })
})
