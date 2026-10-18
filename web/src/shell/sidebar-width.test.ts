import { describe, expect, test } from "vitest"

import {
  clampSidebarWidth,
  keyboardWidth,
  parseStoredWidth,
} from "./sidebar-width"

describe("clampSidebarWidth", () => {
  test("clamps, rounds and defaults", () => {
    expect(clampSidebarWidth(100)).toBe(200)
    expect(clampSidebarWidth(400)).toBe(360)
    expect(clampSidebarWidth(241.6)).toBe(242)
    expect(clampSidebarWidth(Number.NaN)).toBe(240)
    expect(clampSidebarWidth(Number.POSITIVE_INFINITY)).toBe(240)
  })
})

describe("parseStoredWidth", () => {
  test("accepts only whole numbers", () => {
    expect(parseStoredWidth(null)).toBe(240)
    expect(parseStoredWidth("abc")).toBe(240)
    expect(parseStoredWidth("-5")).toBe(240)
    expect(parseStoredWidth("300")).toBe(300)
    expect(parseStoredWidth("999")).toBe(360)
  })
})

describe("keyboardWidth", () => {
  test("steps, jumps and ignores other keys", () => {
    expect(keyboardWidth(240, "ArrowRight", false)).toBe(248)
    expect(keyboardWidth(240, "ArrowRight", true)).toBe(280)
    expect(keyboardWidth(204, "ArrowLeft", false)).toBe(200)
    expect(keyboardWidth(300, "Home", false)).toBe(200)
    expect(keyboardWidth(300, "End", false)).toBe(360)
    expect(keyboardWidth(300, "Enter", false)).toBe(240)
    expect(keyboardWidth(300, "a", false)).toBeNull()
  })
})
