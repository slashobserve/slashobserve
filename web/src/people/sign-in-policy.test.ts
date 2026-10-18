import { describe, expect, test } from "vitest"

import {
  domainList,
  isAllowAll,
  isOnline,
  joinDomains,
  providerLabel,
} from "./sign-in-policy"

describe("providerLabel", () => {
  test("uses the trimmed provider name", () => {
    expect(providerLabel(" Dex ")).toBe("Dex")
  })

  test("falls back when the name is empty or blank", () => {
    expect(providerLabel(undefined)).toBe("your identity provider")
    expect(providerLabel("")).toBe("your identity provider")
    expect(providerLabel("   ")).toBe("your identity provider")
  })
})

describe("domainList", () => {
  test("trims, drops blanks and removes duplicates", () => {
    expect(domainList([" a.com", "", "b.com ", "a.com", "  "])).toEqual([
      "a.com",
      "b.com",
    ])
  })
})

describe("joinDomains", () => {
  test("joins one, two and three domains", () => {
    expect(joinDomains(["a.com"])).toBe("a.com")
    expect(joinDomains(["a.com", "b.com"])).toBe("a.com or b.com")
    expect(joinDomains(["a.com", "b.com", "c.com"])).toBe(
      "a.com, b.com or c.com"
    )
  })
})

describe("isAllowAll", () => {
  test("follows any_email_domain", () => {
    expect(isAllowAll({ any_email_domain: true })).toBe(true)
    expect(isAllowAll({ any_email_domain: false })).toBe(false)
  })
})

describe("isOnline", () => {
  const now = Date.parse("2026-10-19T12:00:00Z")
  const ago = (minutes: number) =>
    new Date(now - minutes * 60_000).toISOString()

  test("is online within two minutes", () => {
    expect(isOnline(ago(1), now)).toBe(true)
    expect(isOnline(ago(2), now)).toBe(true)
  })

  test("is offline after two minutes or without a time", () => {
    expect(isOnline(ago(3), now)).toBe(false)
    expect(isOnline(null, now)).toBe(false)
  })
})
