import type { Threshold } from "@/spec"

export type ThresholdColor = "ok" | "info" | "warning" | "critical" | "custom"

export type ThresholdRow = {
  key: string
  color: ThresholdColor
  hex: string
  value: string
}

export type RowsResult =
  { ok: true; thresholds: Threshold[] | null } | { ok: false; error: string }

export const maxThresholds = 10
const defaultHex = "#2a78d6"
export const valuesError =
  "Values must be numbers that increase from top to bottom."
export const hexError = "Use a color like #2a78d6."

export const thresholdWords: Record<ThresholdColor, string> = {
  ok: "OK",
  info: "Info",
  warning: "Warning",
  critical: "Critical",
  custom: "Custom color",
}

export const thresholdColors = Object.keys(thresholdWords) as ThresholdColor[]

const addOrder = ["warning", "critical", "info", "ok"] as const
const decimalPattern = /^-?(\d+\.?\d*|\.\d+)(e[+-]?\d+)?$/i
const hexPattern = /^#[0-9a-fA-F]{6}$/

let rowCount = 0

function newKey(): string {
  rowCount += 1
  return `threshold-${rowCount}`
}

function isSemantic(color: string): color is (typeof addOrder)[number] {
  return (addOrder as readonly string[]).includes(color)
}

function parseValue(text: string): number | null {
  const trimmed = text.trim()
  if (!decimalPattern.test(trimmed)) return null
  const value = Number(trimmed)
  return Number.isFinite(value) ? value : null
}

export function rowsFromThresholds(
  thresholds: readonly Threshold[] | undefined
): ThresholdRow[] {
  return (thresholds ?? []).map((threshold) => ({
    key: newKey(),
    color: isSemantic(threshold.color) ? threshold.color : "custom",
    hex: isSemantic(threshold.color) ? "" : threshold.color,
    value: threshold.value === undefined ? "" : String(threshold.value),
  }))
}

export function thresholdsFromRows(rows: readonly ThresholdRow[]): RowsResult {
  const values: (number | null)[] = []
  for (const [index, row] of rows.entries()) {
    const text = row.value.trim()
    if (text === "" && index === 0) {
      values.push(null)
      continue
    }
    const value = parseValue(text)
    const previous = values.findLast((item) => item !== null)
    if (value === null || (previous !== undefined && value <= previous)) {
      return { ok: false, error: valuesError }
    }
    values.push(value)
  }
  if (rows.some((row) => row.color === "custom" && !hexPattern.test(row.hex))) {
    return { ok: false, error: hexError }
  }
  if (rows.length === 0) return { ok: true, thresholds: null }
  return {
    ok: true,
    thresholds: rows.map((row, index) => {
      const color = row.color === "custom" ? row.hex : row.color
      const value = values[index]
      return value === null ? { color } : { value, color }
    }),
  }
}

export function setRowColor(
  row: ThresholdRow,
  color: ThresholdColor
): ThresholdRow {
  if (color !== "custom") return { ...row, color, hex: "" }
  return { ...row, color, hex: row.hex === "" ? defaultHex : row.hex }
}

export function addRow(rows: readonly ThresholdRow[]): ThresholdRow[] {
  if (rows.length >= maxThresholds) return [...rows]
  const values = rows
    .map((row) => parseValue(row.value))
    .filter((value): value is number => value !== null)
  const next =
    values.length === 0 ? 0 : Number((Math.max(...values) + 1).toPrecision(12))
  const used = new Set(rows.map((row) => row.color))
  const color = addOrder.find((candidate) => !used.has(candidate))
  const row: ThresholdRow = {
    key: newKey(),
    color: color ?? "custom",
    hex: color ? "" : defaultHex,
    value: String(next),
  }
  return [...rows, row]
}
