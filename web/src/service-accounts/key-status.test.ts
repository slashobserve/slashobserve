import { afterEach, beforeEach, describe, expect, test, vi } from "vitest"

import {
  canRevokeKey,
  expiryOptions,
  expiryState,
  sortKeys,
} from "./key-status"
import type { ServiceAccountKey } from "./queries"

const now = Date.parse("2026-03-10T12:00:00Z")
const day = 86_400_000

function at(offsetMs: number): string {
  return new Date(now + offsetMs).toISOString()
}

function makeKey(
  overrides: Partial<ServiceAccountKey> = {}
): ServiceAccountKey {
  return {
    id: "key-1",
    service_account_id: "account-1",
    name: "ci",
    display_prefix: "sok_abcd",
    created_by: null,
    created_at: at(-30 * day),
    expires_at: null,
    last_used_at: null,
    revoked_at: null,
    state: "active",
    ...overrides,
  }
}

beforeEach(() => {
  vi.useFakeTimers()
  vi.setSystemTime(now)
})

afterEach(() => {
  vi.useRealTimers()
})

describe("expiryOptions", () => {
  test("lists 90 days first, then 30 days, 1 year and never", () => {
    expect(expiryOptions.map((option) => option.value)).toEqual([
      "90d",
      "30d",
      "365d",
      "never",
    ])
  })

  test("labels each option", () => {
    expect(expiryOptions.map((option) => option.label)).toEqual([
      "In 90 days",
      "In 30 days",
      "In 1 year",
      "Never",
    ])
  })
})

describe("expiryState", () => {
  test("reports a revoked key as critical", () => {
    const key = makeKey({
      state: "revoked",
      revoked_at: at(-day),
      expires_at: at(30 * day),
    })
    expect(expiryState(key)).toEqual({ tone: "critical", text: "Revoked" })
  })

  test("reports a key the server marks expired as critical", () => {
    const key = makeKey({ state: "expired", expires_at: at(-day) })
    expect(expiryState(key)).toEqual({ tone: "critical", text: "Expired" })
  })

  test("trusts the server's state over the browser's clock", () => {
    const key = makeKey({ state: "active", expires_at: at(-day) })
    expect(expiryState(key).tone).toBe("ok")
  })

  test("says a key without expiry never expires", () => {
    expect(expiryState(makeKey())).toEqual({
      tone: "ok",
      text: "Never expires",
    })
  })

  test("warns when the server marks the key expiring", () => {
    expect(
      expiryState(makeKey({ state: "expiring", expires_at: at(3 * day) }))
    ).toEqual({ tone: "warning", text: "In 3 days" })
    expect(
      expiryState(makeKey({ state: "expiring", expires_at: at(6.5 * day - 1) }))
    ).toEqual({ tone: "warning", text: "In 6 days" })
  })

  test("is ok for an active key", () => {
    expect(expiryState(makeKey({ expires_at: at(6.5 * day) }))).toEqual({
      tone: "ok",
      text: "In 7 days",
    })
    expect(expiryState(makeKey({ expires_at: at(7 * day) }))).toEqual({
      tone: "ok",
      text: "In 7 days",
    })
    expect(expiryState(makeKey({ expires_at: at(90 * day) }))).toEqual({
      tone: "ok",
      text: "In 90 days",
    })
  })

  test("rounds days to the nearest day instead of flooring", () => {
    expect(expiryState(makeKey({ expires_at: at(90 * day - 5000) })).text).toBe(
      "In 90 days"
    )
    expect(expiryState(makeKey({ expires_at: at(89.6 * day) })).text).toBe(
      "In 90 days"
    )
    expect(expiryState(makeKey({ expires_at: at(89.4 * day) })).text).toBe(
      "In 89 days"
    )
  })

  test("says tomorrow for about one day and counts hours below it", () => {
    expect(expiryState(makeKey({ expires_at: at(day) })).text).toBe("Tomorrow")
    expect(expiryState(makeKey({ expires_at: at(1.4 * day) })).text).toBe(
      "Tomorrow"
    )
    expect(expiryState(makeKey({ expires_at: at(1.5 * day) })).text).toBe(
      "In 2 days"
    )
    expect(
      expiryState(makeKey({ state: "expiring", expires_at: at(day - 1) }))
    ).toEqual({
      tone: "warning",
      text: "In 23 h",
    })
  })

  test("counts days up to two years and switches to years at 730 days", () => {
    expect(expiryState(makeKey({ expires_at: at(729.6 * day) }))).toEqual({
      tone: "ok",
      text: "In 730 days",
    })
    expect(expiryState(makeKey({ expires_at: at(730 * day - 1) })).text).toBe(
      "In 730 days"
    )
    expect(expiryState(makeKey({ expires_at: at(730 * day) })).text).toBe(
      "In 2 years"
    )
  })

  test("falls back to years for a distant expiry", () => {
    expect(expiryState(makeKey({ expires_at: at(800 * day) }))).toEqual({
      tone: "ok",
      text: "In 2 years",
    })
  })

  test("measures against the given time when one is passed", () => {
    const key = makeKey({ expires_at: at(3 * day) })
    expect(expiryState(key, now + 2 * day).text).toBe("Tomorrow")
  })
})

describe("sortKeys", () => {
  test("puts active keys first, then expired, then revoked", () => {
    const revoked = makeKey({
      id: "revoked",
      state: "revoked",
      revoked_at: at(-day),
    })
    const expired = makeKey({
      id: "expired",
      state: "expired",
      expires_at: at(-day),
    })
    const expiring = makeKey({
      id: "expiring",
      state: "expiring",
      expires_at: at(day),
      created_at: at(-40 * day),
    })
    const active = makeKey({ id: "active", expires_at: at(30 * day) })
    const sorted = sortKeys([revoked, expired, expiring, active])
    expect(sorted.map((key) => key.id)).toEqual([
      "active",
      "expiring",
      "expired",
      "revoked",
    ])
  })

  test("orders each group by newest created first", () => {
    const keys = [
      makeKey({ id: "active-old", created_at: at(-10 * day) }),
      makeKey({
        id: "revoked-new",
        state: "revoked",
        created_at: at(-day),
        revoked_at: at(-1),
      }),
      makeKey({ id: "active-new", created_at: at(-2 * day) }),
      makeKey({
        id: "expired-new",
        state: "expired",
        created_at: at(-3 * day),
        expires_at: at(-day),
      }),
      makeKey({
        id: "expired-old",
        state: "expired",
        created_at: at(-20 * day),
        expires_at: at(-day),
      }),
      makeKey({
        id: "revoked-old",
        state: "revoked",
        created_at: at(-5 * day),
        revoked_at: at(-1),
      }),
    ]
    expect(sortKeys(keys).map((key) => key.id)).toEqual([
      "active-new",
      "active-old",
      "expired-new",
      "expired-old",
      "revoked-new",
      "revoked-old",
    ])
  })

  test("does not change the input order", () => {
    const keys = [
      makeKey({ id: "revoked", state: "revoked", revoked_at: at(-day) }),
      makeKey({ id: "active" }),
    ]
    sortKeys(keys)
    expect(keys.map((key) => key.id)).toEqual(["revoked", "active"])
  })
})

describe("canRevokeKey", () => {
  test.each([
    ["active", true],
    ["expiring", true],
    ["expired", false],
    ["revoked", false],
  ] as const)("a %s key: %s", (state, want) => {
    expect(canRevokeKey({ state })).toBe(want)
  })
})
