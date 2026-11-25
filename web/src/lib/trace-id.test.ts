import { describe, expect, it } from "vitest"

import { validTraceId } from "./trace-id"

describe("validTraceId", () => {
  it("accepts 16 and 32 hex digits and lowercases them", () => {
    expect(validTraceId("0AF7651916CD43DD")).toBe("0af7651916cd43dd")
    expect(validTraceId(" ef177585544c8e5e1e23c6ca4d1232bb ")).toBe(
      "ef177585544c8e5e1e23c6ca4d1232bb"
    )
  })

  it.each([
    "",
    "checkout",
    "0".repeat(32),
    "0".repeat(16),
    "abc123",
    "g".repeat(32),
    42,
    null,
    undefined,
  ])("rejects %s", (value) => {
    expect(validTraceId(value)).toBeNull()
  })
})
