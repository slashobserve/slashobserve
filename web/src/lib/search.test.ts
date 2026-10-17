import { describe, expect, test } from "vitest"

import { parseSearch, stringifySearch } from "./search"

describe("parseSearch", () => {
  test("keeps plain values as strings and repeated keys as arrays", () => {
    expect(parseSearch("?from=now-6h&var-pod=a&var-pod=b&n=1")).toEqual({
      from: "now-6h",
      "var-pod": ["a", "b"],
      n: "1",
    })
  })

  test("accepts a string without the leading question mark", () => {
    expect(parseSearch("from=now-1h")).toEqual({ from: "now-1h" })
  })

  test("returns an empty object for an empty string", () => {
    expect(parseSearch("")).toEqual({})
    expect(parseSearch("?")).toEqual({})
  })

  test("decodes percent escapes and plus signs", () => {
    expect(parseSearch("?q=a%20b&r=c+d")).toEqual({ q: "a b", r: "c d" })
  })

  test("keeps three or more repeats in order", () => {
    expect(parseSearch("?v=a&v=b&v=c")).toEqual({ v: ["a", "b", "c"] })
  })
})

describe("stringifySearch", () => {
  test("writes plain values and repeats array keys", () => {
    expect(
      stringifySearch({ from: "now-6h", "var-pod": ["a", "b"], n: 1 })
    ).toBe("?from=now-6h&var-pod=a&var-pod=b&n=1")
  })

  test("skips undefined and null values", () => {
    expect(stringifySearch({ a: undefined, b: null, c: "x" })).toBe("?c=x")
  })

  test("returns an empty string when nothing remains", () => {
    expect(stringifySearch({})).toBe("")
    expect(stringifySearch({ a: undefined })).toBe("")
  })

  test("escapes reserved characters", () => {
    expect(stringifySearch({ q: "a&b=c" })).toBe("?q=a%26b%3Dc")
  })

  test("round trips what it writes", () => {
    const search = { from: "now-6h", "var-pod": ["a", "b"], n: "1" }
    expect(parseSearch(stringifySearch(search))).toEqual(search)
  })
})
