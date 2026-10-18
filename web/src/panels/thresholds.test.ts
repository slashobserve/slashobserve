import { describe, expect, it } from "vitest"

import { commonDecimals } from "@/lib/units"
import type { Unit } from "@/spec"

import { thresholdFor } from "./thresholds"

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
