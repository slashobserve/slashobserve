import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { isLinkGoneError } from "./queries"

const token = "pub_abc"
const panelKey = ["public", token, "versions", 3, "panels", "p1", {}]

describe("isLinkGoneError", () => {
  it("is true for a not_found answer to a panel read of this link", () => {
    expect(
      isLinkGoneError(panelKey, token, new ApiError(404, "not_found", "gone"))
    ).toBe(true)
  })

  it("is false for a panel the dashboard does not have", () => {
    expect(
      isLinkGoneError(
        panelKey,
        token,
        new ApiError(404, "panel_not_found", "no panel")
      )
    ).toBe(false)
  })

  it("is false for other links, other queries and other errors", () => {
    const gone = new ApiError(404, "not_found", "gone")
    expect(isLinkGoneError(["public", "other", "x"], token, gone)).toBe(false)
    expect(isLinkGoneError(["public", token], token, gone)).toBe(false)
    expect(isLinkGoneError(["dashboards", token, "x"], token, gone)).toBe(false)
    expect(
      isLinkGoneError(panelKey, token, new ApiError(500, "internal", "x"))
    ).toBe(false)
  })
})
