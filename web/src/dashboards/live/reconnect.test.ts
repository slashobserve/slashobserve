import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import {
  afterClosed,
  backoffDelay,
  goneOutcome,
  isGone,
  shouldCheckDashboard,
} from "./reconnect"

describe("backoffDelay", () => {
  it("spreads each attempt between half and all of the base delay", () => {
    expect(backoffDelay(0, () => 0)).toBe(500)
    expect(backoffDelay(0, () => 0.5)).toBe(750)
    expect(backoffDelay(0, () => 1)).toBe(1_000)
    expect(backoffDelay(3, () => 0)).toBe(4_000)
    expect(backoffDelay(3, () => 1)).toBe(8_000)
  })

  it("caps the base delay at 30 seconds", () => {
    expect(backoffDelay(5, () => 1)).toBe(30_000)
    expect(backoffDelay(60, () => 0)).toBe(15_000)
    expect(backoffDelay(60, () => 1)).toBe(30_000)
  })

  it("stays inside the jitter window with the real random source", () => {
    for (let attempt = 0; attempt < 8; attempt++) {
      const base = Math.min(1_000 * 2 ** attempt, 30_000)
      for (let run = 0; run < 50; run++) {
        const delay = backoffDelay(attempt)
        expect(delay).toBeGreaterThanOrEqual(base / 2)
        expect(delay).toBeLessThanOrEqual(base)
      }
    }
  })
})

describe("afterClosed", () => {
  it("reopens at once when the server ended the stream at its lifetime", () => {
    expect(afterClosed('{"reason":"lifetime"}')).toBe("reopen")
  })

  it("stops when the caller lost access", () => {
    expect(afterClosed('{"reason":"access_lost"}')).toBe("stop")
  })

  it.each([
    '{"reason":"stopping"}',
    '{"reason":"check_failed"}',
    '{"reason":"slow_subscriber"}',
    '{"reason":"read_failed"}',
    '{"reason":"a reason added later"}',
    '{"reason":7}',
    "{}",
    "null",
    "not json",
  ])("retries with a delay after %s", (data) => {
    expect(afterClosed(data)).toBe("retry")
  })
})

describe("shouldCheckDashboard", () => {
  it("checks on every third consecutive failure", () => {
    const checked = Array.from({ length: 10 }, (_, index) => index).filter(
      shouldCheckDashboard
    )
    expect(checked).toEqual([3, 6, 9])
  })
})

describe("isGone", () => {
  it("treats a removed dashboard and a lost folder as gone for the open page", () => {
    expect(isGone(new ApiError(404, "not_found", "gone"))).toBe(true)
    expect(isGone(new ApiError(403, "forbidden_role", "no access"))).toBe(true)
  })

  it("keeps the page for a server hiccup", () => {
    expect(isGone(new ApiError(502, "datasource_unavailable", "down"))).toBe(
      false
    )
  })
})

describe("goneOutcome", () => {
  it("says deleted only when the server announced the deletion and the dashboard is missing", () => {
    expect(goneOutcome(new ApiError(404, "not_found", "gone"), true)).toBe(
      "deleted"
    )
  })

  it("lets the page reload when a missing dashboard was not announced as deleted", () => {
    expect(goneOutcome(new ApiError(404, "not_found", "gone"), false)).toBe(
      "reload"
    )
  })

  it("lets the page reload for a lost folder", () => {
    expect(goneOutcome(new ApiError(403, "forbidden_role", "no"), true)).toBe(
      "reload"
    )
  })

  it("keeps the page for a server hiccup", () => {
    expect(goneOutcome(new ApiError(502, "down", "down"), false)).toBe("keep")
  })
})
