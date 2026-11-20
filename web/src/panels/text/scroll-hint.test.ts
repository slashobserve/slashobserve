import { describe, expect, it } from "vitest"

import { hasMoreBelow } from "./scroll-hint"

describe("hasMoreBelow", () => {
  it("is true while content is cut off at the bottom", () => {
    expect(
      hasMoreBelow({ scrollHeight: 120, clientHeight: 29, scrollTop: 0 })
    ).toBe(true)
    expect(
      hasMoreBelow({ scrollHeight: 120, clientHeight: 29, scrollTop: 60 })
    ).toBe(true)
  })

  it("is false when everything fits or the end is reached", () => {
    expect(
      hasMoreBelow({ scrollHeight: 29, clientHeight: 29, scrollTop: 0 })
    ).toBe(false)
    expect(
      hasMoreBelow({ scrollHeight: 120, clientHeight: 29, scrollTop: 91 })
    ).toBe(false)
  })

  it("ignores a rounding pixel", () => {
    expect(
      hasMoreBelow({ scrollHeight: 100, clientHeight: 50, scrollTop: 49 })
    ).toBe(false)
  })
})
