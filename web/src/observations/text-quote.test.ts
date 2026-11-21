import { describe, expect, it, vi } from "vitest"

import { findQuotes, quoteOf, textAnchor, textAnchors } from "./text-quote"

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

  it("never splits a character outside the Basic Multilingual Plane", () => {
    const quote = quoteOf("a" + "🔥".repeat(250))
    expect(Array.from(quote)).toHaveLength(200)
    expect(quote).not.toMatch(/[\ud800-\udbff]$/)
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

describe("findQuotes", () => {
  it("finds a quote that spans two paragraphs", () => {
    const root = rootWith("The error rate rose", "after the deploy.")
    const [range] = findQuotes(root, "rate rose\n\nafter the")
    expect(range?.toString()).toBe("rate roseafter the")
  })

  it("finds nothing once the text is gone", () => {
    const root = rootWith("The error rate fell.")
    expect(findQuotes(root, "rate rose")).toEqual([])
    expect(findQuotes(root, "   ")).toEqual([])
  })
})

describe("textAnchors.locate", () => {
  it("places a repeated quote at the occurrence nearest the stored spot", () => {
    const root = rootWith(
      "p99 rose to 2s.",
      "Rollback done.",
      "p99 rose to 2s."
    )
    const surface = document.createElement("div")
    surface.dataset.canvasSurface = ""
    surface.append(root)
    document.body.append(surface)
    const paragraphs = Array.from(root.querySelectorAll("p"))
    vi.spyOn(Range.prototype, "getClientRects").mockImplementation(function (
      this: Range
    ) {
      const row = paragraphs.findIndex((each) =>
        each.contains(this.startContainer)
      )
      return [
        DOMRect.fromRect({ x: 0, y: row * 20, width: 100, height: 16 }),
      ] as unknown as DOMRectList
    })
    vi.spyOn(HTMLElement.prototype, "offsetWidth", "get").mockReturnValue(200)
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockReturnValue(60)
    const locate = textAnchors.locate as unknown as (
      layout: unknown,
      anchor: unknown
    ) => { y: number } | null
    const anchor = (y: number) => ({
      kind: "text",
      pos: { x: 0, y },
      quote: "p99 rose to 2s.",
    })
    expect(locate({ root, markdown: "" }, anchor(56 / 60))?.y).toBe(40)
    expect(locate({ root, markdown: "" }, anchor(16 / 60))?.y).toBe(0)
    vi.restoreAllMocks()
  })

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
