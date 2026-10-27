import { describe, expect, it } from "vitest"

import { lineDiff } from "./line-diff"

describe("lineDiff", () => {
  it("marks identical text as unchanged", () => {
    expect(lineDiff("a\nb", "a\nb")).toEqual([
      { kind: "same", text: "a" },
      { kind: "same", text: "b" },
    ])
  })

  it("shows a changed line as a removal then an addition", () => {
    expect(lineDiff("a\nold\nc", "a\nnew\nc")).toEqual([
      { kind: "same", text: "a" },
      { kind: "del", text: "old" },
      { kind: "add", text: "new" },
      { kind: "same", text: "c" },
    ])
  })

  it("shows an added line", () => {
    expect(lineDiff("a\nc", "a\nb\nc")).toEqual([
      { kind: "same", text: "a" },
      { kind: "add", text: "b" },
      { kind: "same", text: "c" },
    ])
  })

  it("shows a removed line", () => {
    expect(lineDiff("a\nb\nc", "a\nc")).toEqual([
      { kind: "same", text: "a" },
      { kind: "del", text: "b" },
      { kind: "same", text: "c" },
    ])
  })

  it("treats everything as added when before is empty", () => {
    expect(lineDiff("", "a\nb")).toEqual([
      { kind: "add", text: "a" },
      { kind: "add", text: "b" },
    ])
  })

  it("treats everything as removed when after is empty", () => {
    expect(lineDiff("a", "")).toEqual([{ kind: "del", text: "a" }])
  })

  it("falls back to all removed and all added beyond 400 lines", () => {
    const long = Array.from({ length: 401 }, (_, i) => `l${i}`).join("\n")
    const diff = lineDiff(long, `${long}\nextra`)
    expect(diff.some((line) => line.kind === "same")).toBe(false)
    expect(diff.filter((line) => line.kind === "del")).toHaveLength(401)
    expect(diff.filter((line) => line.kind === "add")).toHaveLength(402)
  })
})
