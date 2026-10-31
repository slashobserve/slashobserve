import { describe, expect, it } from "vitest"

import { commonDecimals } from "@/lib/units"
import type { Unit } from "@/spec"

import { isSpecColor, thresholdColor, thresholdFor } from "./thresholds"

describe("thresholdFor labels", () => {
  const label = (
    values: number[],
    value: number,
    threshold: number,
    unit?: Unit
  ) =>
    thresholdFor(
      value,
      [{ value: threshold, color: "purple" }],
      unit,
      commonDecimals(values, unit)
    ).label

  it("keeps a fractional threshold next to a large value", () => {
    expect(label([1234], 1234, 0.5)).toBe("At or above 0.5")
    expect(label([1234], 1234, 0.5, "percent")).toBe("At or above 0.5%")
  })

  it("keeps the threshold's own precision next to a close value", () => {
    expect(label([99.9, 100], 100, 99.95)).toBe("At or above 99.95")
  })

  it("widens to the shared decimals when those are larger", () => {
    expect(label([12.345], 12.345, 10)).toBe("At or above 10.00")
  })

  it("does not round a large threshold to the significant figure cap", () => {
    expect(label([1000.5], 1000.5, 1000.5)).toBe("At or above 1,000.5")
    expect(label([12345.5], 12345.5, 12345.5)).toBe("At or above 12,345.5")
  })

  it("keeps the precision a unit scaled threshold needs", () => {
    expect(
      label([15_425_000_000], 15_425_000_000, 15_425_000_000, "decbytes")
    ).toBe("At or above 15.425 GB")
  })

  it("falls back to the threshold's own precision without shared decimals", () => {
    expect(commonDecimals([Infinity])).toBeUndefined()
    expect(label([Infinity], Infinity, 0.125)).toBe("At or above 0.125")
    expect(label([Infinity], Infinity, 15_425_000_000, "decbytes")).toBe(
      "At or above 15.425 GB"
    )
  })

  it("ignores float noise in a threshold", () => {
    expect(label([1], 1, 0.1 + 0.2)).toBe("At or above 0.3")
  })

  it("never labels the semantic colors with a number", () => {
    expect(
      thresholdFor(5, [{ value: 1, color: "critical" }], undefined, 2).label
    ).toBe("Critical")
  })
})

describe("thresholdFor base step", () => {
  it("treats a first step without a value as the base below every other step", () => {
    const steps = [{ color: "ok" }, { value: 10, color: "critical" }]
    expect(thresholdFor(-5, steps).level).toBe("ok")
    expect(thresholdFor(9, steps).level).toBe("ok")
    expect(thresholdFor(10, steps).level).toBe("critical")
  })

  it("labels a custom base step as the base", () => {
    expect(thresholdFor(1, [{ color: "#336699" }]).label).toBe("Base")
  })
})

describe("the text color", () => {
  it("resolves to the foreground token and the others to their threshold tokens", () => {
    expect(thresholdColor("text")).toBe("var(--foreground)")
    expect(thresholdColor("critical")).toBe("var(--threshold-critical)")
    expect(thresholdColor("#112233")).toBe("#112233")
  })

  it("colors a threshold step with the foreground token", () => {
    expect(
      thresholdFor(5, [{ color: "ok" }, { value: 1, color: "text" }])
    ).toMatchObject({ level: "custom", color: "var(--foreground)" })
  })

  it("accepts the semantic colors, text and six digit hex only", () => {
    for (const color of [
      "ok",
      "info",
      "warning",
      "critical",
      "text",
      "#0a1B2c",
    ]) {
      expect(isSpecColor(color)).toBe(true)
    }
    for (const color of ["gray", "Text", "#fff", "field", "", 3, null]) {
      expect(isSpecColor(color)).toBe(false)
    }
  })
})
