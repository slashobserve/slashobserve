import { describe, expect, it } from "vitest"

import { lowerFirst } from "./text"

describe("lowerFirst", () => {
  it("lowercases the first character only", () => {
    expect(lowerFirst("Yesterday")).toBe("yesterday")
    expect(lowerFirst("In 74 days")).toBe("in 74 days")
    expect(lowerFirst("")).toBe("")
  })
})
