import { describe, expect, it } from "vitest"

import {
  addRow,
  hexError,
  maxThresholds,
  rowsFromThresholds,
  setRowColor,
  thresholdsFromRows,
  valuesError,
  type ThresholdRow,
} from "./threshold-rows"

function row(
  color: ThresholdRow["color"],
  value: string,
  hex = ""
): ThresholdRow {
  return { key: `${color}-${value}`, color, hex, value }
}

describe("rowsFromThresholds", () => {
  it("turns steps into editable rows", () => {
    const rows = rowsFromThresholds([
      { color: "ok" },
      { value: 0.3, color: "warning" },
      { value: 0.5, color: "#2a78d6" },
    ])
    expect(
      rows.map(({ color, hex, value }) => ({ color, hex, value }))
    ).toEqual([
      { color: "ok", hex: "", value: "" },
      { color: "warning", hex: "", value: "0.3" },
      { color: "custom", hex: "#2a78d6", value: "0.5" },
    ])
  })

  it("gives every row its own key", () => {
    const rows = rowsFromThresholds([
      { value: 1, color: "warning" },
      { value: 2, color: "critical" },
    ])
    expect(new Set(rows.map((item) => item.key)).size).toBe(2)
  })

  it("has no rows without thresholds", () => {
    expect(rowsFromThresholds(undefined)).toEqual([])
  })
})

describe("thresholdsFromRows", () => {
  it("round trips the canvas thresholds", () => {
    const thresholds = [
      { value: 0.3, color: "warning" },
      { value: 0.5, color: "critical" },
    ]
    expect(thresholdsFromRows(rowsFromThresholds(thresholds))).toEqual({
      ok: true,
      thresholds,
    })
  })

  it("keeps an empty first value as the base step", () => {
    expect(thresholdsFromRows([row("ok", ""), row("warning", "80")])).toEqual({
      ok: true,
      thresholds: [{ color: "ok" }, { value: 80, color: "warning" }],
    })
  })

  it("accepts a custom color", () => {
    expect(thresholdsFromRows([row("custom", "5", "#A1b2C3")])).toEqual({
      ok: true,
      thresholds: [{ value: 5, color: "#A1b2C3" }],
    })
  })

  it("refuses values that do not increase", () => {
    const result = { ok: false, error: valuesError }
    expect(
      thresholdsFromRows([row("warning", "0.5"), row("critical", "0.2")])
    ).toEqual(result)
    expect(
      thresholdsFromRows([row("warning", "0.5"), row("critical", "0.5")])
    ).toEqual(result)
  })

  it("refuses an empty value after the first row", () => {
    expect(
      thresholdsFromRows([
        row("ok", "1"),
        row("warning", ""),
        row("critical", "3"),
      ])
    ).toEqual({ ok: false, error: valuesError })
  })

  it("refuses text that is not a number", () => {
    expect(thresholdsFromRows([row("warning", "abc")])).toEqual({
      ok: false,
      error: valuesError,
    })
    expect(thresholdsFromRows([row("warning", "0x10")])).toEqual({
      ok: false,
      error: valuesError,
    })
  })

  it("refuses a bad hex color", () => {
    expect(thresholdsFromRows([row("custom", "1", "#12345")])).toEqual({
      ok: false,
      error: hexError,
    })
    expect(thresholdsFromRows([row("custom", "1", "")])).toEqual({
      ok: false,
      error: hexError,
    })
  })

  it("gives no thresholds when every row is removed", () => {
    expect(thresholdsFromRows([])).toEqual({ ok: true, thresholds: null })
  })
})

describe("setRowColor", () => {
  it("clears the hex for a semantic color and defaults it for a custom one", () => {
    const custom = setRowColor(row("warning", "1"), "custom")
    expect(custom).toMatchObject({ color: "custom", hex: "#2a78d6" })
    expect(setRowColor(custom, "info")).toMatchObject({
      color: "info",
      hex: "",
    })
  })
})

describe("addRow", () => {
  it("starts with a warning at 0", () => {
    const rows = addRow([])
    expect(rows).toHaveLength(1)
    expect(rows[0]).toMatchObject({ color: "warning", value: "0" })
  })

  it("adds the first unused color one above the largest value", () => {
    const rows = addRow([row("warning", "0.3")])
    expect(rows[1]).toMatchObject({ color: "critical", value: "1.3" })
  })

  it("skips colors already used and ignores unreadable values", () => {
    const rows = addRow([
      row("warning", "2"),
      row("critical", "x"),
      row("info", ""),
    ])
    expect(rows[3]).toMatchObject({ color: "ok", value: "3" })
  })

  it("falls back to a custom color when every named color is used", () => {
    const rows = addRow([
      row("ok", "1"),
      row("info", "2"),
      row("warning", "3"),
      row("critical", "4"),
    ])
    expect(rows[4]).toMatchObject({
      color: "custom",
      hex: "#2a78d6",
      value: "5",
    })
  })

  it("keeps float noise out of the new value", () => {
    expect(addRow([row("warning", "0.57")])[1].value).toBe("1.57")
  })

  it("adds nothing at the limit", () => {
    const full = Array.from({ length: maxThresholds }, (_, index) =>
      row("custom", String(index), "#000000")
    )
    expect(addRow(full)).toHaveLength(maxThresholds)
  })
})
