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

describe("plainText Markdown details", () => {
  it("drops paired emphasis but keeps lone underscores and stars", () => {
    expect(plainText("*slow* _late_ a_b_c 3 * 2 * 4")).toBe(
      "slow late a_b_c 3 * 2 * 4"
    )
  })

  it("drops stacked and ordered markers, and image syntax", () => {
    expect(plainText("> ## Cause\n1. step ![chart](/c.png)")).toBe(
      "Cause\nstep chart"
    )
  })

  it("allows one level of parentheses in a link address", () => {
    expect(plainText("[page](https://x.test/a_(b)) ok")).toBe("page ok")
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

  it("keeps a dash, quote or star that begins a later segment of a sentence", () => {
    const out = plainSegments([
      { text: "timeout", match: true },
      { text: " - retrying after 5s > 3 * 2", match: false },
    ])
    expect(out.map((segment) => segment.text).join("")).toBe(
      "timeout - retrying after 5s > 3 * 2"
    )
  })

  it("still drops a list marker after a line break in a later segment", () => {
    const out = plainSegments([
      { text: "cause", match: true },
      { text: "\n- slow query", match: false },
    ])
    expect(out[1]?.text).toBe("\nslow query")
  })
})
