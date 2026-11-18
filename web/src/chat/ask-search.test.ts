import { describe, expect, test } from "vitest"

import { parseAskSearch } from "./ask-search"

describe("parseAskSearch", () => {
  test("keeps a prompt", () => {
    expect(parseAskSearch({ prompt: "why is checkout slow" })).toEqual({
      prompt: "why is checkout slow",
    })
  })

  test.each([
    ["missing", {}],
    ["empty", { prompt: "" }],
    ["blank", { prompt: "  \n " }],
    ["a list", { prompt: ["a", "b"] }],
  ])("drops a %s prompt", (_name, search) => {
    expect(parseAskSearch(search)).toEqual({})
  })

  test("trims and cuts the prompt to the limit", () => {
    expect(parseAskSearch({ prompt: `  ${"a".repeat(5000)}  ` })).toEqual({
      prompt: "a".repeat(4000),
    })
  })

  test("reads a numeric prompt as text", () => {
    expect(parseAskSearch({ prompt: 42 })).toEqual({ prompt: "42" })
  })
})
