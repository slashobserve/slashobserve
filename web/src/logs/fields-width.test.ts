import { afterEach, describe, expect, it } from "vitest"

import {
  clampFieldsWidth,
  fieldsDefaultWidth,
  fieldsMaxWidth,
  fieldsMinWidth,
  fieldsWidthKey,
  keyboardFieldsWidth,
  parseStoredFieldsWidth,
  storedFieldsWidth,
  storeFieldsWidth,
} from "./fields-width"

afterEach(() => localStorage.clear())

describe("clampFieldsWidth", () => {
  it("keeps a width inside the limits", () => {
    expect(clampFieldsWidth(10)).toBe(fieldsMinWidth)
    expect(clampFieldsWidth(5000)).toBe(fieldsMaxWidth)
    expect(clampFieldsWidth(300.4)).toBe(300)
    expect(clampFieldsWidth(Number.NaN)).toBe(fieldsDefaultWidth)
  })
})

describe("parseStoredFieldsWidth", () => {
  it("falls back to the default for anything but whole pixels", () => {
    expect(parseStoredFieldsWidth(null)).toBe(fieldsDefaultWidth)
    expect(parseStoredFieldsWidth("wide")).toBe(fieldsDefaultWidth)
    expect(parseStoredFieldsWidth("-5")).toBe(fieldsDefaultWidth)
    expect(parseStoredFieldsWidth("320")).toBe(320)
    expect(parseStoredFieldsWidth("99999")).toBe(fieldsMaxWidth)
  })
})

describe("storeFieldsWidth", () => {
  it("round-trips a width through localStorage", () => {
    storeFieldsWidth(400)
    expect(localStorage.getItem(fieldsWidthKey)).toBe("400")
    expect(storedFieldsWidth()).toBe(400)
  })
})

describe("keyboardFieldsWidth", () => {
  it("steps with the arrows and jumps with Home, End and Enter", () => {
    expect(keyboardFieldsWidth(300, "ArrowRight", false)).toBe(308)
    expect(keyboardFieldsWidth(300, "ArrowLeft", true)).toBe(260)
    expect(keyboardFieldsWidth(300, "Home", false)).toBe(fieldsMinWidth)
    expect(keyboardFieldsWidth(300, "End", false)).toBe(fieldsMaxWidth)
    expect(keyboardFieldsWidth(300, "Enter", false)).toBe(fieldsDefaultWidth)
    expect(keyboardFieldsWidth(300, "a", false)).toBeNull()
  })
})
