import { describe, expect, it } from "vitest"

import {
  entryHint,
  entryPlaceholder,
  enumDefaultLabel,
  parseOptionEntry,
} from "./values"

describe("parseOptionEntry", () => {
  it("clears the option on an empty entry", () => {
    expect(parseOptionEntry("decimals", "  ")).toEqual({
      ok: true,
      value: undefined,
    })
  })

  it("keeps numbers inside the schema's range", () => {
    expect(parseOptionEntry("decimals", "3")).toEqual({ ok: true, value: 3 })
    expect(parseOptionEntry("decimals", "11").ok).toBe(false)
    expect(parseOptionEntry("decimals", "1.5").ok).toBe(false)
    expect(parseOptionEntry("min", "-2.5")).toEqual({ ok: true, value: -2.5 })
    expect(parseOptionEntry("linearThreshold", "0").ok).toBe(false)
    expect(parseOptionEntry("barWidthFactor", "0.5")).toEqual({
      ok: true,
      value: 0.5,
    })
  })

  it("checks text against the schema's pattern and length", () => {
    expect(parseOptionEntry("timeFrom", "now-6h")).toEqual({
      ok: true,
      value: "now-6h",
    })
    expect(parseOptionEntry("timeFrom", "6h").ok).toBe(false)
    expect(parseOptionEntry("stackGroup", "x".repeat(51)).ok).toBe(false)
    expect(parseOptionEntry("color", "warning").ok).toBe(true)
    expect(parseOptionEntry("color", "#12ab3f").ok).toBe(true)
    expect(parseOptionEntry("color", "red").ok).toBe(false)
  })
})

describe("entry hints", () => {
  it("shows the Spec default as the placeholder", () => {
    expect(entryPlaceholder("lineWidth")).toBe("2")
    expect(entryPlaceholder("decimals")).toBe("Auto")
    expect(entryPlaceholder("timeFrom")).toBe("now-1h")
  })

  it("names what a field accepts", () => {
    expect(entryHint("decimals")).toBe("A whole number from 0 to 10.")
    expect(entryHint("linearThreshold")).toBe("A number above 0.")
    expect(entryHint("min")).toBe("A number.")
    expect(entryHint("timeShift")).toBe("For example 1h.")
    expect(entryHint("noValue")).toBe("At most 100 characters.")
  })

  it("labels the default choice of an enum", () => {
    expect(enumDefaultLabel("lineInterpolation")).toBe("Default (Linear)")
    expect(enumDefaultLabel("colorMode")).toBe("Default")
  })
})
