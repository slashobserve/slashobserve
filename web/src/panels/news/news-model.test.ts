import { describe, expect, it } from "vitest"

import { ApiError } from "@/api/errors"

import { isNotAllowed, newsHref, retryNews } from "./news-model"

describe("newsHref", () => {
  it.each([
    ["https://example.com/a?b=1#c", "https://example.com/a?b=1#c"],
    ["http://example.com/a", "http://example.com/a"],
    [null, null],
    ["javascript:alert(1)", null],
    ["data:text/html,<p>x</p>", null],
    ["ftp://example.com/a", null],
    ["/relative/path", null],
    ["//example.com/a", null],
    ["", null],
    ["not a url", null],
  ])("maps %j to %j", (link, expected) => {
    expect(newsHref(link)).toBe(expected)
  })
})

describe("isNotAllowed", () => {
  it("is true only for an outbound_domain_not_allowed ApiError", () => {
    const blocked = new ApiError(
      403,
      "outbound_domain_not_allowed",
      "example.com is not on the outbound allowlist."
    )
    expect(isNotAllowed(blocked)).toBe(true)
    expect(isNotAllowed(new ApiError(403, "forbidden", "Forbidden"))).toBe(
      false
    )
    expect(isNotAllowed(new ApiError(502, "feed_unavailable", "Down"))).toBe(
      false
    )
    expect(isNotAllowed(new Error("outbound_domain_not_allowed"))).toBe(false)
    expect(isNotAllowed(null)).toBe(false)
  })
})

describe("retryNews", () => {
  it("never retries a 4xx answer", () => {
    for (const status of [400, 403, 404, 429]) {
      expect(retryNews(0, new ApiError(status, "x", "x"))).toBe(false)
    }
  })

  it("retries a 5xx answer or a network failure at most twice", () => {
    const bad = new ApiError(502, "feed_unavailable", "Down")
    expect(retryNews(0, bad)).toBe(true)
    expect(retryNews(1, bad)).toBe(true)
    expect(retryNews(2, bad)).toBe(false)
    expect(retryNews(1, new TypeError("network"))).toBe(true)
    expect(retryNews(2, new TypeError("network"))).toBe(false)
  })
})
