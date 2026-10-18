import { describe, expect, test } from "vitest"

import { domainsText, effortName, providerName, sessionLength } from "./format"

describe("providerName", () => {
  test("names the known providers", () => {
    expect(providerName("anthropic")).toBe("Anthropic")
    expect(providerName("openai")).toBe("OpenAI-compatible")
    expect(providerName("replay")).toBe("Replay")
  })

  test("keeps an unknown provider as is", () => {
    expect(providerName("bedrock")).toBe("bedrock")
  })

  test("says when no provider is set", () => {
    expect(providerName(null)).toBe("Not set")
  })
})

describe("effortName", () => {
  test("capitalises the level", () => {
    expect(effortName("medium")).toBe("Medium")
  })

  test("spells out xhigh", () => {
    expect(effortName("xhigh")).toBe("Extra high")
  })

  test("says when the provider has no effort", () => {
    expect(effortName(null)).toBe("Not used by this provider")
  })
})

describe("sessionLength", () => {
  test("counts whole days", () => {
    expect(sessionLength(604800)).toBe("7 days")
    expect(sessionLength(86400)).toBe("1 day")
  })

  test("counts whole hours", () => {
    expect(sessionLength(43200)).toBe("12 hours")
  })

  test("falls back to minutes", () => {
    expect(sessionLength(300)).toBe("5 minutes")
  })
})

describe("domainsText", () => {
  test("says any domain when everyone may sign in", () => {
    expect(
      domainsText({ allowed_email_domains: [], any_email_domain: true })
    ).toBe("Any domain")
  })

  test("lists the allowed domains", () => {
    expect(
      domainsText({
        allowed_email_domains: ["a.com", "b.com"],
        any_email_domain: false,
      })
    ).toBe("a.com, b.com")
  })
})
