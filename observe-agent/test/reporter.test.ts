import { describe, expect, test } from "bun:test"
import { chunkText } from "../src/reporter"

describe("chunkText", () => {
  test("splits at code points, never inside a surrogate pair", () => {
    const text = "\u{1f600}".repeat(5)
    expect(chunkText(text, 2)).toEqual([
      "\u{1f600}\u{1f600}",
      "\u{1f600}\u{1f600}",
      "\u{1f600}",
    ])
  })

  test("gives no chunk for empty text and one for short text", () => {
    expect(chunkText("", 4000)).toEqual([])
    expect(chunkText("abc", 4000)).toEqual(["abc"])
  })

  test("an oversized turn text is sent in chunks of at most 4000 code points", () => {
    const chunks = chunkText("a".repeat(9000), 4000)
    expect(chunks.map((c) => c.length)).toEqual([4000, 4000, 1000])
  })
})
