import { describe, expect, it } from "vitest"

import { viewsText } from "./views"

describe("viewsText", () => {
  it("names the window and the plural", () => {
    expect(viewsText(0, 30)).toBe("0 views in 30 days")
    expect(viewsText(1, 30)).toBe("1 view in 30 days")
    expect(viewsText(12, 30)).toBe("12 views in 30 days")
  })
})
