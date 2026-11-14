import { describe, expect, test } from "vitest"

import { homeRedirect, homeSearch, parseHomeTab } from "./home-tab"

describe("parseHomeTab", () => {
  test.each([
    ["observe", "observe"],
    ["import", "import"],
    ["ask", "ask"],
    [undefined, "ask"],
    ["bogus", "ask"],
    [1, "ask"],
  ])("reads %j as %s", (value, tab) => {
    expect(parseHomeTab(value)).toBe(tab)
  })
})

describe("homeSearch", () => {
  test("leaves the address bare for the ask tab", () => {
    expect(homeSearch("ask")).toEqual({})
    expect(homeSearch("observe")).toEqual({ tab: "observe" })
    expect(homeSearch("import")).toEqual({ tab: "import" })
  })
})

describe("homeRedirect", () => {
  test.each([
    ["", null],
    ["?tab=import", null],
    ["?tab=bogus", null],
    [
      "?from=now-1h&var-pod=a&var-pod=b",
      "/ask?from=now-1h&var-pod=a&var-pod=b",
    ],
    ["?tab=import&q=x", "/ask?tab=import&q=x"],
  ])("sends %j to %j", (searchStr, href) => {
    expect(homeRedirect(searchStr)).toBe(href)
  })
})
