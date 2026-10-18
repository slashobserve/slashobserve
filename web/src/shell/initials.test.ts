import { describe, expect, test } from "vitest"

import { initials } from "./initials"

describe("initials", () => {
  test("uses the name first", () => {
    expect(initials("Ishan")).toBe("IS")
    expect(initials("admin")).toBe("AD")
    expect(initials("Priya Shah")).toBe("PS")
  })

  test("falls back to the email local part", () => {
    expect(initials("", "jane.doe@example.com")).toBe("JD")
  })

  test("shows a question mark when there is nothing", () => {
    expect(initials("", "")).toBe("?")
  })
})
