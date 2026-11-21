import { describe, expect, it } from "vitest"

import { keyBlockOf, takesBlockComments } from "./observation-canvas"

const place = { canComment: true, arranging: false, editingBlock: null }
const panel = { id: "b1", kind: "panel", state: "live" } as const
const text = { id: "b2", kind: "text", state: "live" } as const
const run = { id: "b3", kind: "run", state: "live" } as const
const requested = { id: "b4", kind: "panel", state: "requested" } as const

describe("takesBlockComments", () => {
  it("puts live panel and text blocks on the canvas", () => {
    expect(takesBlockComments(panel, place)).toBe(true)
    expect(takesBlockComments(text, place)).toBe(true)
    expect(takesBlockComments(run, place)).toBe(false)
    expect(takesBlockComments(requested, place)).toBe(false)
  })

  it("takes nothing without Comment or while the page is arranged", () => {
    expect(takesBlockComments(panel, { ...place, canComment: false })).toBe(
      false
    )
    expect(takesBlockComments(panel, { ...place, arranging: true })).toBe(false)
  })

  it("leaves out the block being edited", () => {
    expect(takesBlockComments(panel, { ...place, editingBlock: "b1" })).toBe(
      false
    )
    expect(takesBlockComments(text, { ...place, editingBlock: "b1" })).toBe(
      true
    )
  })
})

describe("keyBlockOf", () => {
  const blocks = [panel, text, run]

  it("takes the first candidate that is a panel block on the canvas", () => {
    expect(keyBlockOf(blocks, ["b2", "b1"], place)).toBe("b1")
    expect(keyBlockOf(blocks, [undefined, "b1"], place)).toBe("b1")
  })

  it("names no block when none of them takes a whole-block comment", () => {
    expect(keyBlockOf(blocks, ["b2", "b3", null], place)).toBeUndefined()
    expect(
      keyBlockOf(blocks, ["b1"], { ...place, editingBlock: "b1" })
    ).toBeUndefined()
  })
})
