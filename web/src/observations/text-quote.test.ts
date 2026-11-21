import { describe, expect, it } from "vitest"

import { findQuote, quoteOf, textAnchor, textAnchors } from "./text-quote"

function rootWith(...paragraphs: string[]): HTMLElement {
  const root = document.createElement("div")
  for (const text of paragraphs) {
    const paragraph = document.createElement("p")
    paragraph.textContent = text
    root.append(paragraph)
  }
  document.body.append(root)
  return root
}

describe("quoteOf", () => {
  it("folds whitespace and keeps at most 200 characters", () => {
    expect(quoteOf("  card\n\n declined  ")).toBe("card declined")
    expect(Array.from(quoteOf("x".repeat(300)))).toHaveLength(200)
  })

  it("gives nothing for a selection of only whitespace", () => {
    expect(quoteOf(" \n\t ")).toBe("")
  })
})

describe("textAnchor", () => {
  it("points at the selected text from where it starts in the block", () => {
    expect(
      textAnchor("card declined", { x: 50, y: 20 }, { width: 200, height: 80 })
    ).toEqual({
      kind: "text",
      pos: { x: 0.25, y: 0.25 },
      quote: "card declined",
    })
  })
})

describe("findQuote", () => {
  it("finds a quote that spans two paragraphs", () => {
    const root = rootWith("The error rate rose", "after the deploy.")
    const range = findQuote(root, "rate rose\n\nafter the")
    expect(range?.toString()).toBe("rate roseafter the")
  })

  it("finds nothing once the text is gone", () => {
    const root = rootWith("The error rate fell.")
    expect(findQuote(root, "rate rose")).toBeNull()
    expect(findQuote(root, "   ")).toBeNull()
  })
})

describe("textAnchors.locate", () => {
  it("does not place a text anchor whose quote is no longer there", () => {
    const root = rootWith("Something else entirely.")
    const locate = textAnchors.locate as unknown as (
      layout: unknown,
      anchor: unknown
    ) => unknown
    expect(
      locate(
        { root, markdown: "" },
        { kind: "text", pos: { x: 0, y: 0 }, quote: "rate rose" }
      )
    ).toBeNull()
  })
})
