import { describe, expect, it } from "vitest"

import { decodeRison, RisonError } from "./rison"

describe("decodeRison", () => {
  it("decodes a Kibana Discover state", () => {
    expect(
      decodeRison(
        `(filters:!(),query:(language:kuery,query:'service.name : "checkout"'),columns:!(message,log.level))`
      )
    ).toEqual({
      filters: [],
      query: { language: "kuery", query: 'service.name : "checkout"' },
      columns: ["message", "log.level"],
    })
  })

  it("decodes the literals, numbers and bare time expressions", () => {
    expect(
      decodeRison("(a:!t,b:!f,c:!n,d:-12.5,e:3e2,f:now-15m,g:'')")
    ).toEqual({
      a: true,
      b: false,
      c: null,
      d: -12.5,
      e: 300,
      f: "now-15m",
      g: "",
    })
  })

  it("decodes escapes in strings", () => {
    expect(decodeRison("'it!'s !!'")).toBe("it's !")
  })

  it("decodes nested arrays and empty containers", () => {
    expect(decodeRison("!(!(1,2),!(),())")).toEqual([[1, 2], [], {}])
  })

  it("decodes quoted keys", () => {
    expect(decodeRison("('a b':1)")).toEqual({ "a b": 1 })
  })

  it.each([
    ["an unterminated string", "(a:'open)", 3],
    ["a missing colon", "(a 1)", 2],
    ["text after the value", "(a:1)x", 5],
    ["an unknown escape", "'a!x'", 2],
    ["an unclosed object", "(a:1", 4],
    ["an unknown ! form", "!x", 0],
    ["no value", "", 0],
  ])("throws with the offset for %s", (_name, text, offset) => {
    let error: unknown
    try {
      decodeRison(text)
    } catch (thrown) {
      error = thrown
    }
    expect(error).toBeInstanceOf(RisonError)
    expect((error as RisonError).offset).toBe(offset)
  })

  it("refuses a value nested deeper than 32 levels", () => {
    expect(() => decodeRison("!(".repeat(40) + ")".repeat(40))).toThrow(
      RisonError
    )
  })
})
