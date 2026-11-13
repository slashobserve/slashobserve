// @vitest-environment node
import { describe, expect, it } from "vitest"

import { parseMemory, rememberIn } from "./datasource-memory"

describe("parseMemory", () => {
  it("reads lists most recent first", () => {
    expect(
      parseMemory(JSON.stringify({ logs: ["a", "b"], traces: ["t"] }))
    ).toEqual({ logs: ["a", "b"], traces: ["t"] })
  })

  it("reads the old single uid as a one entry list", () => {
    expect(
      parseMemory(JSON.stringify({ logs: "ch", traces: "tempo" }))
    ).toEqual({ logs: ["ch"], traces: ["tempo"] })
  })

  it("leaves out a kind that was never stored", () => {
    expect(parseMemory(JSON.stringify({ logs: ["a"] }))).toEqual({
      logs: ["a"],
      traces: [],
    })
  })

  it("drops entries that are not strings", () => {
    expect(parseMemory(JSON.stringify({ logs: ["a", 3, null, "b"] }))).toEqual({
      logs: ["a", "b"],
      traces: [],
    })
  })

  it.each([null, "", "not json", "[]", "3", '{"logs": 3}'])(
    "reads %j as empty lists",
    (raw) => {
      expect(parseMemory(raw)).toEqual({ logs: [], traces: [] })
    }
  )

  it("keeps at most 20 uids", () => {
    const many = Array.from({ length: 30 }, (_, index) => `s${index}`)
    expect(parseMemory(JSON.stringify({ logs: many })).logs).toEqual(
      many.slice(0, 20)
    )
  })
})

describe("rememberIn", () => {
  it("puts the uid first", () => {
    expect(rememberIn(["a", "b"], "c")).toEqual(["c", "a", "b"])
  })

  it("moves a known uid to the front without repeating it", () => {
    expect(rememberIn(["a", "b", "c"], "c")).toEqual(["c", "a", "b"])
  })

  it("keeps at most 20 uids, dropping the oldest", () => {
    const full = Array.from({ length: 20 }, (_, index) => `s${index}`)
    const next = rememberIn(full, "new")
    expect(next).toHaveLength(20)
    expect(next[0]).toBe("new")
    expect(next).not.toContain("s19")
  })
})
