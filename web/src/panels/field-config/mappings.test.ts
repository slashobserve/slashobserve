import { describe, expect, it } from "vitest"

import type { Mapping } from "@/spec"

import { displayValue, fieldRange, type FieldConfig } from "."

const config = (extra: Partial<FieldConfig> = {}): FieldConfig => ({
  thresholdMode: "absolute",
  style: "line",
  fill: 0,
  lineStyle: "solid",
  lineInterpolation: "linear",
  lineWidth: 2,
  points: "auto",
  pointSize: 4,
  spanNulls: false,
  stack: "none",
  axis: "left",
  hidden: false,
  scale: "linear",
  logBase: 10,
  axisCenteredZero: false,
  axisBorder: false,
  axisColor: "text",
  mappings: [],
  links: [],
  ...extra,
})

const show = (value: unknown, mappings: Mapping[], extra = {}) =>
  displayValue(value, config({ mappings, ...extra }))

describe("value mappings", () => {
  it("maps a value on its string and numeric forms", () => {
    const mappings: Mapping[] = [{ value: "1", text: "Up", color: "ok" }]
    expect(show("1", mappings)).toMatchObject({
      text: "Up",
      mapped: true,
      color: "var(--threshold-ok)",
    })
    expect(show(1, mappings)).toMatchObject({ text: "Up", mapped: true })
    expect(show(1.5, mappings)).toMatchObject({ text: "1.5", mapped: false })
  })

  it("takes the first mapping that matches", () => {
    const mappings: Mapping[] = [
      { value: "1", text: "first" },
      { value: "1", text: "second" },
    ]
    expect(show(1, mappings).text).toBe("first")
  })

  it("maps an inclusive range that is open on one side", () => {
    const mappings: Mapping[] = [
      { range: [null, 10], text: "low" },
      { range: [90, null], text: "high" },
      { range: [10, 20], text: "mid" },
    ]
    expect(show(-5, mappings).text).toBe("low")
    expect(show(10, mappings).text).toBe("low")
    expect(show(20, mappings).text).toBe("mid")
    expect(show(90, mappings).text).toBe("high")
    expect(show(1e9, mappings).text).toBe("high")
    expect(show(50, mappings)).toMatchObject({ text: "50", mapped: false })
  })

  it("maps a number-like string into a range as parseFloat reads it", () => {
    const mappings: Mapping[] = [{ range: [500, 599], text: "server error" }]
    expect(show("503", mappings)).toMatchObject({
      text: "server error",
      mapped: true,
    })
    expect(show("404", mappings).mapped).toBe(false)
    expect(show("abc", mappings)).toMatchObject({ text: "abc", mapped: false })
  })

  it("matches a regex and substitutes capture groups", () => {
    const mappings: Mapping[] = [{ regex: "(\\d+)ms", text: "$1 millis" }]
    expect(show("42ms", mappings)).toMatchObject({
      text: "42 millis",
      mapped: true,
    })
    expect(show("42s", mappings)).toMatchObject({ text: "42s", mapped: false })
  })

  it("anchors a regex unless it is written with slashes", () => {
    expect(show("a42", [{ regex: "\\d+", text: "n" }]).mapped).toBe(false)
    expect(show("a42", [{ regex: "/\\d+/", text: "n" }]).mapped).toBe(true)
  })

  it("maps the same value every time with a sticky regex", () => {
    const mappings: Mapping[] = [{ regex: "/up/y", text: "Up" }]
    expect([1, 2, 3].map(() => show("up", mappings).text)).toEqual([
      "Up",
      "Up",
      "Up",
    ])
  })

  it("never matches an invalid regex", () => {
    expect(show("a", [{ regex: "(", text: "x" }]).mapped).toBe(false)
  })

  it("keeps the value text for a regex mapping that has only a color", () => {
    expect(show("err", [{ regex: "err", color: "critical" }])).toMatchObject({
      text: "err",
      color: "var(--threshold-critical)",
      mapped: true,
    })
  })

  it("maps the special null, nan and empty values", () => {
    const mappings: Mapping[] = [
      { special: "null", text: "no data" },
      { special: "nan", text: "not a number" },
      { special: "empty", text: "blank" },
    ]
    expect(show(null, mappings).text).toBe("no data")
    expect(show(undefined, mappings).text).toBe("no data")
    expect(show(NaN, mappings).text).toBe("not a number")
    expect(show("", mappings).text).toBe("blank")
  })

  it("maps the special true and false for booleans and their strings", () => {
    const mappings: Mapping[] = [
      { special: "true", text: "Yes" },
      { special: "false", text: "No" },
    ]
    expect(show(true, mappings).text).toBe("Yes")
    expect(show("true", mappings).text).toBe("Yes")
    expect(show(false, mappings).text).toBe("No")
    expect(show("false", mappings).text).toBe("No")
  })

  it("shows the plain value when no mapping matches", () => {
    expect(show(true, [])).toMatchObject({ text: "true", mapped: false })
    expect(show("abc", [])).toMatchObject({ text: "abc", numeric: null })
  })

  it("formats an unmapped number with the unit and decimals", () => {
    const result = show(1234.5678, [{ value: "0", text: "zero" }], {
      unit: "percent",
      decimals: 1,
    })
    expect(result).toMatchObject({
      text: "1,234.6%",
      numeric: 1234.5678,
      mapped: false,
    })
  })

  it("uses the shared decimals only when the config sets none", () => {
    expect(
      displayValue(2.4, config({ unit: "percent" }), { decimals: 2 }).text
    ).toBe("2.40%")
    expect(
      displayValue(2.4, config({ unit: "percent", decimals: 0 }), {
        decimals: 2,
      }).text
    ).toBe("2%")
  })

  it("shows noValue, else a dash, for an absent value", () => {
    expect(displayValue(null, config()).text).toBe("-")
    expect(displayValue(NaN, config()).text).toBe("-")
    expect(displayValue("", config()).text).toBe("-")
    expect(displayValue(null, config({ noValue: "n/a" }))).toMatchObject({
      text: "n/a",
      numeric: null,
      mapped: false,
    })
  })

  it("lets a special mapping without text keep noValue and set a color", () => {
    const result = show(null, [{ special: "null", color: "warning" }], {
      noValue: "gone",
    })
    expect(result).toMatchObject({
      text: "gone",
      color: "var(--threshold-warning)",
      mapped: true,
    })
  })
})

describe("value color and thresholds", () => {
  const thresholds = [
    { color: "ok" },
    { value: 50, color: "warning" },
    { value: 90, color: "critical" },
  ]

  it("takes the threshold color over the fixed color", () => {
    const result = displayValue(95, config({ thresholds, color: "#123456" }))
    expect(result.color).toBe("var(--threshold-critical)")
    expect(result.threshold.level).toBe("critical")
  })

  it("falls back to the fixed color, then to null", () => {
    expect(displayValue(5, config({ color: "#123456" })).color).toBe("#123456")
    expect(displayValue(5, config({ color: "info" })).color).toBe(
      "var(--threshold-info)"
    )
    expect(displayValue(5, config()).color).toBeNull()
  })

  it("lets a mapping color beat the threshold color", () => {
    const result = show(95, [{ range: [90, null], color: "info" }], {
      thresholds,
    })
    expect(result.color).toBe("var(--threshold-info)")
    expect(result.threshold.level).toBe("critical")
  })

  it("uses the base step below every other threshold", () => {
    const result = displayValue(1, config({ thresholds }))
    expect(result.threshold.level).toBe("ok")
    expect(result.color).toBe("var(--threshold-ok)")
  })

  it("reads percentage thresholds against the configured min and max", () => {
    const percentage = config({
      thresholds,
      thresholdMode: "percentage",
      min: 0,
      max: 200,
    })
    expect(displayValue(120, percentage).threshold.level).toBe("warning")
    expect(displayValue(190, percentage).threshold.level).toBe("critical")
  })

  it("reads percentage thresholds against the field's own range without config", () => {
    const percentage = config({ thresholds, thresholdMode: "percentage" })
    const options = { fieldMin: 100, fieldMax: 200 }
    expect(displayValue(160, percentage, options).threshold.level).toBe(
      "warning"
    )
    expect(displayValue(195, percentage, options).threshold.level).toBe(
      "critical"
    )
    expect(displayValue(100, percentage, options).threshold.level).toBe("ok")
  })

  it("treats equal min and max as 100 percent", () => {
    const percentage = config({ thresholds, thresholdMode: "percentage" })
    const result = displayValue(7, percentage, { fieldMin: 7, fieldMax: 7 })
    expect(result.threshold.level).toBe("critical")
  })

  it("has no threshold in percentage mode without a range", () => {
    const percentage = config({ thresholds, thresholdMode: "percentage" })
    expect(displayValue(7, percentage).threshold.level).toBe("base")
  })

  it("has no threshold for text values", () => {
    expect(displayValue("abc", config({ thresholds })).threshold.level).toBe(
      "base"
    )
  })
})

describe("fieldRange", () => {
  it("returns the finite min and max of a number field", () => {
    expect(
      fieldRange({
        name: "v",
        type: "number",
        values: [3, null, NaN, -2, 8, Infinity],
      })
    ).toEqual({ min: -2, max: 8 })
  })

  it("is empty for other fields and for no finite values", () => {
    expect(fieldRange({ name: "s", type: "string", values: ["a"] })).toEqual({})
    expect(fieldRange({ name: "v", type: "number", values: [null] })).toEqual({
      min: undefined,
      max: undefined,
    })
  })
})
