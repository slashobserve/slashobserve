import { describe, expect, it } from "vitest"

import { folderLabel } from "./folder-label"

describe("folderLabel", () => {
  it("calls a personal folder My Dashboards", () => {
    expect(folderLabel({ personal: true, title: "alice" })).toBe(
      "My Dashboards"
    )
  })

  it("uses the title of a shared folder", () => {
    expect(folderLabel({ personal: false, title: "Platform" })).toBe("Platform")
  })
})
