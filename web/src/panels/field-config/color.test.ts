import { describe, expect, it } from "vitest"

import type { WireField } from "@/panels/frames"
import type { DataPanel } from "@/spec"

import { displayValue, effectiveColorMode, type FieldConfig } from "."
import { createFieldResolver } from "./resolve"

function configOf(extra: Partial<DataPanel>): FieldConfig {
  const panel = {
    id: "p1",
    title: "Panel",
    gridPos: { x: 0, y: 0, w: 12, h: 8 },
    type: "stat",
    datasource: "prom",
    queries: [{ query: "up" }],
    ...extra,
  } as DataPanel
  const field: WireField = { name: "Value", type: "number", values: [1] }
  const frame = { ref: "A", fields: [field] }
  return createFieldResolver(panel).configFor(frame, field)
}

describe("effectiveColorMode", () => {
  it.each([
    [{}, "palette"],
    [{ color: "critical" as const }, "fixed"],
    [{ color: "critical" as const, colorMode: "palette" as const }, "palette"],
    [{ colorMode: "scheme" as const }, "scheme"],
  ])("derives the mode of %j as %s", (props, mode) => {
    expect(effectiveColorMode(props)).toBe(mode)
  })
})

describe("scheme colors in displayValue", () => {
  const scheme = {
    colorMode: "scheme" as const,
    colorScheme: "yellowRed" as const,
  }

  it("colors a value by its place in the field range", () => {
    const config = configOf(scheme)
    const shown = displayValue(100, config, { fieldMin: 0, fieldMax: 100 })
    expect(shown.color).toBe("var(--threshold-critical)")
    expect(shown.threshold.level).toBe("base")
  })

  it("prefers the configured minimum and maximum over the data range", () => {
    const config = configOf({ ...scheme, min: 0, max: 400 })
    const shown = displayValue(100, config, { fieldMin: 0, fieldMax: 100 })
    expect(shown.color).toBe(
      "color-mix(in oklab, var(--threshold-critical) 25%, var(--threshold-warning))"
    )
  })

  it("lets the scheme win over thresholds, and a mapping win over both", () => {
    const config = configOf({
      ...scheme,
      thresholds: [{ color: "ok" }, { value: 50, color: "info" }],
      mappings: [{ value: "100", text: "Full", color: "warning" }],
    })
    const options = { fieldMin: 0, fieldMax: 100 }
    const scaled = displayValue(60, config, options)
    expect(scaled.color).toBe(
      "color-mix(in oklab, var(--threshold-critical) 60%, var(--threshold-warning))"
    )
    expect(scaled.threshold.level).toBe("info")
    expect(displayValue(100, config, options).color).toBe(
      "var(--threshold-warning)"
    )
  })

  it("draws no scheme color without a range, a scheme or a number", () => {
    expect(displayValue(5, configOf(scheme), {}).color).toBeNull()
    expect(
      displayValue(5, configOf({ colorMode: "scheme" }), {
        fieldMin: 0,
        fieldMax: 10,
      }).color
    ).toBeNull()
    expect(
      displayValue(null, configOf(scheme), { fieldMin: 0, fieldMax: 10 }).color
    ).toBeNull()
  })

  it("keeps the thresholds when the mode is not a scheme", () => {
    const config = configOf({
      colorScheme: "yellowRed",
      thresholds: [{ color: "ok" }, { value: 50, color: "info" }],
    })
    const shown = displayValue(60, config, { fieldMin: 0, fieldMax: 100 })
    expect(shown.color).toBe("var(--threshold-info)")
  })

  it("ignores a stored fixed color once the mode is reset to the palette", () => {
    const fixed = configOf({ color: "critical" })
    expect(displayValue(1, fixed).color).toBe("var(--threshold-critical)")
    const reset = configOf({ color: "critical", colorMode: "palette" })
    expect(displayValue(1, reset).color).toBeNull()
  })
})
