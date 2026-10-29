import { describe, expect, test } from "vitest"

import { ApiError, NetworkError } from "@/api/errors"

import {
  domainDraftError,
  domainProblemText,
  matchText,
} from "./outbound-domains"

describe("domainDraftError", () => {
  test.each([
    ["", null],
    ["   ", null],
    ["example.com", null],
    ["*.example.com", null],
    ["  example.com  ", null],
    ["https://example.com", "Enter a domain, not a URL: example.com"],
    ["ftp://example.com/feed", "Enter a domain, not a URL: example.com"],
    [
      "example.com/feed",
      "Enter only the domain name, like example.com or *.example.com.",
    ],
    [
      "example.com:443",
      "Enter only the domain name, like example.com or *.example.com.",
    ],
    [
      "me@example.com",
      "Enter only the domain name, like example.com or *.example.com.",
    ],
    [
      "exa mple.com",
      "Enter only the domain name, like example.com or *.example.com.",
    ],
  ])("checks %j", (text, expected) => {
    expect(domainDraftError(text)).toBe(expected)
  })
})

describe("domainProblemText", () => {
  test.each([
    [409, "domain_exists", "x", "That domain is already allowed."],
    [
      422,
      "too_many_domains",
      "x",
      "The list holds at most 100 domains. Remove one first.",
    ],
    [
      409,
      "managed_by_config",
      "x",
      "This list is set by SLASHOBSERVE_OUTBOUND_DOMAINS on the server.",
    ],
    [422, "invalid_domain", "label too long", "Label too long."],
    [404, "not_found", "not found", "That domain was already removed."],
    [500, "internal", "boom", "Boom."],
  ])("explains %i %s", (status, code, message, expected) => {
    expect(domainProblemText(new ApiError(status, code, message))).toBe(
      expected
    )
  })

  test("says the server is unreachable on a network failure", () => {
    expect(domainProblemText(new NetworkError("offline"))).toBe(
      "Could not reach the server. Check your connection and try again."
    )
  })

  test("falls back for an unknown failure", () => {
    expect(domainProblemText(new Error("x"))).toBe(
      "Something went wrong. Try again."
    )
  })
})

describe("matchText", () => {
  test("describes an exact host", () => {
    expect(matchText("example.com")).toBe("Only example.com")
  })

  test("describes a wildcard as subdomains only", () => {
    expect(matchText("*.example.com")).toBe("Subdomains of example.com")
  })
})
