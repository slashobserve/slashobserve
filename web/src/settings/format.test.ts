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

  test("counts whole minutes", () => {
    expect(sessionLength(300)).toBe("5 minutes")
    expect(sessionLength(60)).toBe("1 minute")
    expect(sessionLength(5400)).toBe("90 minutes")
  })

  test("keeps the seconds that do not make a whole minute", () => {
    expect(sessionLength(330)).toBe("5 minutes 30 seconds")
    expect(sessionLength(620)).toBe("10 minutes 20 seconds")
    expect(sessionLength(61)).toBe("1 minute 1 second")
    expect(sessionLength(45)).toBe("45 seconds")
  })

  test("keeps the hours in a length with stray seconds", () => {
    expect(sessionLength(3690)).toBe("1 hour 1 minute 30 seconds")
    expect(sessionLength(7201)).toBe("2 hours 1 second")
    expect(sessionLength(90061)).toBe("1 day 1 hour 1 minute 1 second")
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

  test("de-duplicates and ignores blank domains", () => {
    expect(
      domainsText({
        allowed_email_domains: ["a.com", " a.com ", ""],
        any_email_domain: false,
      })
    ).toBe("a.com")
  })

  test("says when no domain is set", () => {
    expect(
      domainsText({ allowed_email_domains: [], any_email_domain: false })
    ).toBe("Not set")
    expect(
      domainsText({ allowed_email_domains: [" "], any_email_domain: false })
    ).toBe("Not set")
  })
})
