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

  test("keeps the dashboard to revise only when it is an id", () => {
    const id = "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11"
    expect(parseAskSearch({ prompt: "add a row", dashboard: id })).toEqual({
      prompt: "add a row",
      dashboard: id,
    })
    expect(
      parseAskSearch({ prompt: "add a row", dashboard: "../../etc" })
    ).toEqual({ prompt: "add a row" })
  })

  test("ignores a dashboard with no prompt", () => {
    expect(
      parseAskSearch({ dashboard: "2f0f2c1e-6a53-4a35-8a0e-1d3b0d1f7a11" })
    ).toEqual({})
  })

  test("reads a numeric prompt as text", () => {
    expect(parseAskSearch({ prompt: 42 })).toEqual({ prompt: "42" })
  })
})
