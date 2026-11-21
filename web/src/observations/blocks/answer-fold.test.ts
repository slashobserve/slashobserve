import { describe, expect, it } from "vitest"

import { foldAnswer } from "./answer-fold"

const filler = "word ".repeat(40).trim()

describe("foldAnswer", () => {
  it("keeps the heading and the first paragraph open and folds the rest", () => {
    const markdown = `## Checkout 5xx\n\n${filler}\n\n${filler}\n\n- one\n- two\n\n${filler}`
    expect(foldAnswer(markdown)).toEqual({
      opening: `## Checkout 5xx\n\n${filler}`,
      rest: `${filler}\n\n- one\n- two\n\n${filler}`,
    })
  })

  it("keeps a short answer whole", () => {
    expect(foldAnswer("## Title\n\nOne.\n\nTwo.\n\nThree.")).toBeNull()
  })

  it("keeps a long answer of one paragraph whole", () => {
    expect(foldAnswer(`## Title\n\n${"word ".repeat(200)}`)).toBeNull()
  })

  it("keeps a long answer without a heading open on its first paragraph", () => {
    const markdown = [filler, filler, filler, filler, "last"].join("\n\n")
    const fold = foldAnswer(markdown)
    expect(fold?.opening).toBe(filler)
    expect(fold?.rest.endsWith("last")).toBe(true)
  })

  it("does not split a paragraph in a code fence at a blank line", () => {
    const code = "```\nfirst\n\nsecond\n```"
    const markdown = `## Title\n\n${code}\n\n${filler.repeat(20)}`
    expect(foldAnswer(markdown)?.opening).toBe(`## Title\n\n${code}`)
  })

  it("treats consecutive headings as the opening", () => {
    const markdown = `# One\n\n## Two\n\n${filler}\n\n${filler.repeat(15)}`
    expect(foldAnswer(markdown)?.opening).toBe(`# One\n\n## Two\n\n${filler}`)
  })

  it("keeps a list with a blank line and an indented continuation together", () => {
    const list = "- one\n\n  still one\n\n- two"
    const markdown = `## Title\n\n${list}\n\n${filler.repeat(20)}`
    const fold = foldAnswer(markdown)
    expect(fold?.opening).toBe(`## Title\n\n${list}`)
    expect(fold?.rest).toBe(filler.repeat(20))
  })

  it("keeps reference link definitions with both parts", () => {
    const markdown = `## Title\n\nSee [the deploy][d].\n\n${filler.repeat(5)} [again][d]\n\n[d]: https://example.com/deploy`
    const fold = foldAnswer(markdown)
    expect(fold?.opening).toBe(
      "## Title\n\nSee [the deploy][d].\n\n[d]: https://example.com/deploy"
    )
    expect(fold?.rest).toBe(
      `${filler.repeat(5)} [again][d]\n\n[d]: https://example.com/deploy`
    )
  })

  it("closes a fence only with the same marker", () => {
    const code = "```\nfirst\n~~~\n\nsecond\n```"
    const markdown = `## Title\n\n${code}\n\n${filler.repeat(20)}`
    expect(foldAnswer(markdown)?.opening).toBe(`## Title\n\n${code}`)
  })
})
