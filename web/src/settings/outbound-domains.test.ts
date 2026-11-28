import { describe, expect, test } from "vitest"

import { ApiError, NetworkError, problemText } from "@/api/errors"

import {
  domainDraftError,
  domainProblems,
  matchText,
  originText,
  removeConsequence,
  type OutboundDomain,
} from "./outbound-domains"

function entry(overrides: Partial<OutboundDomain>): OutboundDomain {
  return {
    domain: "grafana.example.com",
    added_at: "2026-11-18T10:00:00Z",
    added_by: "Ada",
    from_connection: false,
    used_by: [],
    ...overrides,
  }
}

describe("removeConsequence", () => {
  test("names the webhooks, connections and datasources that lose the domain", () => {
    const text = removeConsequence(
      entry({
        used_by: [
          { kind: "webhook", name: "Deploys" },
          { kind: "grafana_connection", name: "Prod" },
          { kind: "datasource", name: "Metrics" },
          { kind: "mcp_source", name: "Docs" },
        ],
      })
    )
    expect(text).toContain(
      "Webhook Deploys, Grafana connection Prod, Datasource Metrics, MCP source Docs"
    )
    expect(text).toContain("can no longer reach it")
  })

  test("warns about on-demand fetches when nothing is listed", () => {
    expect(removeConsequence(entry({}))).toBe(
      "Dashboard news feeds, actions and assets that fetch from it stop loading."
    )
  })
})

describe("originText", () => {
  test("says a connection added the entry", () => {
    expect(originText(entry({ from_connection: true }))).toBe(
      "Added for a connection or datasource"
    )
  })

  test("names the person, or none when they are gone", () => {
    expect(originText(entry({}))).toBe("Added by Ada")
    expect(originText(entry({ added_by: null }))).toBe("Added")
  })

  test("has no origin for an entry set by configuration", () => {
    expect(originText(entry({ added_at: null, added_by: null }))).toBeNull()
  })
})

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
    expect(
      problemText(new ApiError(status, code, message), domainProblems)
    ).toBe(expected)
  })

  test("says the server is unreachable on a network failure", () => {
    expect(problemText(new NetworkError("offline"), domainProblems)).toBe(
      "Could not reach the server. Check your connection and try again."
    )
  })

  test("falls back for an unknown failure", () => {
    expect(problemText(new Error("x"), domainProblems)).toBe(
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
