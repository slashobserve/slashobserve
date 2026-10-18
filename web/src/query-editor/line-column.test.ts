import { describe, expect, it } from "vitest"

import { byteRangeToIndices } from "./byte-offsets"
import { lineAndColumn } from "./line-column"

describe("lineAndColumn", () => {
  it("counts from one on the first line", () => {
    expect(lineAndColumn("sum(rate(", 0)).toEqual({ line: 1, column: 1 })
    expect(lineAndColumn("sum(rate(", 9)).toEqual({ line: 1, column: 10 })
  })

  it("restarts the column after a newline", () => {
    expect(lineAndColumn("up\n  + x", 6)).toEqual({ line: 2, column: 4 })
  })

  it("counts code points, not UTF-16 units", () => {
    const text = "😀é) bad"
    const index = text.indexOf(")")
    expect(index).toBe(3)
    expect(lineAndColumn(text, index)).toEqual({ line: 1, column: 3 })
    const second = "a\n😀😀)"
    expect(lineAndColumn(second, second.indexOf(")"))).toEqual({
      line: 2,
      column: 3,
    })
  })

  it("places a server byte offset on the right column", () => {
    const text = 'up{job="é😀"} )'
    const bytes = new TextEncoder().encode(text.slice(0, text.indexOf(")")))
    const { from } = byteRangeToIndices(text, bytes.length, bytes.length + 1)
    expect(lineAndColumn(text, from)).toEqual({ line: 1, column: 14 })
  })

  it("clamps offsets outside the text", () => {
    expect(lineAndColumn("ab", -3)).toEqual({ line: 1, column: 1 })
    expect(lineAndColumn("ab", 99)).toEqual({ line: 1, column: 3 })
  })
})
