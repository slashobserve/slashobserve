import { describe, expect, it } from "vitest"

import { plainSegments, plainText } from "./plain-text"

describe("plainText", () => {
  it("drops bold, code and strike markers but keeps snake_case words", () => {
    expect(plainText("**p99** of `http_requests_total` ~~was~~ high")).toBe(
      "p99 of http_requests_total was high"
    )
  })

  it("keeps the text of a link and drops its address", () => {
    expect(plainText("see [the dashboard](/d/abc) now")).toBe(
      "see the dashboard now"
    )
  })

  it("drops heading, quote and list markers at the start of a line", () => {
    expect(plainText("## Cause\n- slow query\n> noted")).toBe(
      "Cause\nslow query\nnoted"
    )
  })
})

describe("plainSegments", () => {
  it("cleans each segment and keeps which ones matched", () => {
    expect(
      plainSegments([
        { text: "the `checkout` ", match: false },
        { text: "**errors**", match: true },
      ])
    ).toEqual([
      { text: "the checkout ", match: false },
      { text: "errors", match: true },
    ])
  })
})
