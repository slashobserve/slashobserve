import { describe, expect, it } from "vitest"

import { cleanText, lineRule, textError, textRule } from "./user-text"

describe("cleanText", () => {
  it("trims spaces and zero-width characters at the ends only", () => {
    expect(cleanText("  QA lead  ")).toBe("QA lead")
    expect(cleanText("​QA​ ")).toBe("QA")
    expect(cleanText("QA​x")).toBe("QA​x")
    expect(cleanText("​")).toBe("")
  })
})

describe("textError", () => {
  const title = lineRule(100)

  it("accepts plain text, Hebrew and emoji joiners", () => {
    expect(textError("title", "Checkout", title)).toBeNull()
    expect(textError("title", "שלום", title)).toBeNull()
    expect(textError("title", "Team \u{1f469}‍\u{1f4bb}", title)).toBeNull()
  })

  it("says what to change for each refusal, as the server does", () => {
    const cases: [string, string][] = [
      ["", "Title must not be empty; enter 1 to 100 characters."],
      ["   ", "Title must not be empty; enter 1 to 100 characters."],
      [
        "​",
        "Title has no visible character; enter 1 to 100 characters of visible text.",
      ],
      [
        "a\nb",
        "Title must be one line without control characters; remove line breaks, tabs and other control characters.",
      ],
      [
        "a\u0007b",
        "Title must be one line without control characters; remove line breaks, tabs and other control characters.",
      ],
      [
        "QA-r2-‮evil",
        "Title must not contain bidirectional override characters (U+202A to U+202E, U+2066 to U+2069); remove them.",
      ],
      [
        "QA​x",
        "Title must not contain zero-width or hidden characters; remove them.",
      ],
      ["a".repeat(101), "Title must be at most 100 characters; shorten it."],
    ]
    for (const [raw, message] of cases) {
      expect(textError("title", raw, title)).toBe(message)
    }
  })

  it("counts characters, not UTF-16 units", () => {
    expect(textError("title", "\u{1f600}".repeat(100), title)).toBeNull()
    expect(textError("title", "\u{1f600}".repeat(101), title)).not.toBeNull()
  })

  it("lets text keep line breaks and tabs but not other control characters", () => {
    const body = textRule(4000)
    expect(textError("body", "a\n\tb", body)).toBeNull()
    expect(textError("body", "a\u0007b", body)).toBe(
      "Body must not contain control characters other than line breaks and tabs; remove them."
    )
    expect(textError("body", "a​b", body)).toBeNull()
    expect(textError("body", "​", body)).toContain("no visible character")
  })
})
