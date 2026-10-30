import { readableText, type Rgb } from "../../contrast"
import type { FieldConfig } from "../../field-config"

export type CellType = NonNullable<FieldConfig["cell"]>["type"]

export type CellSpec = {
  type: CellType
  gaugeStyle: "basic" | "lcd"
  valueMode: "color" | "text" | "hidden"
  wholeRow: boolean
  wrap: boolean
  imageAlt?: string
  imageTitle?: string
}

const seriesHues = 8
const maxPills = 20
const jsonSummaryLength = 80

const tintedTypes: ReadonlySet<CellType> = new Set([
  "text",
  "background",
  "gauge",
  "sparkline",
])

export function cellSpecOf(cell: FieldConfig["cell"]): CellSpec {
  return {
    type: cell?.type ?? "auto",
    gaugeStyle: cell?.gaugeStyle ?? "basic",
    valueMode: cell?.valueMode ?? "text",
    wholeRow: cell?.wholeRow ?? false,
    wrap: cell?.wrap ?? false,
    imageAlt: cell?.imageAlt,
    imageTitle: cell?.imageTitle,
  }
}

export function usesFixedColor(type: CellType): boolean {
  return tintedTypes.has(type)
}

export function fillTextColor(
  color: string | null,
  toRgb: (color: string) => Rgb | null
): string | undefined {
  if (color === null) return undefined
  const rgb = toRgb(color)
  return rgb ? readableText(rgb) : undefined
}

export function pillLabels(value: unknown): string[] {
  if (value === null || value === undefined) return []
  const labels = String(value)
    .split(",")
    .map((label) => label.trim())
    .filter((label) => label !== "")
  return [...new Set(labels)].slice(0, maxPills)
}

export function pillHue(label: string): number {
  let hash = 0
  for (const char of label)
    hash = (hash * 31 + (char.codePointAt(0) ?? 0)) >>> 0
  return (hash % seriesHues) + 1
}

export type JsonView = { summary: string; pretty: string }

function parsedJson(value: unknown): unknown {
  if (typeof value !== "string") return value
  const trimmed = value.trim()
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return undefined
  try {
    return JSON.parse(trimmed) as unknown
  } catch {
    return undefined
  }
}

export function jsonView(value: unknown): JsonView | null {
  const parsed = parsedJson(value)
  if (typeof parsed !== "object" || parsed === null) return null
  const compact = JSON.stringify(parsed)
  const summary =
    compact.length > jsonSummaryLength
      ? `${compact.slice(0, jsonSummaryLength - 1)}…`
      : compact
  return { summary, pretty: JSON.stringify(parsed, null, 2) }
}

export function numberSeries(value: unknown): (number | null)[] | null {
  if (!Array.isArray(value)) return null
  const series = value.map((item): number | null =>
    typeof item === "number" && Number.isFinite(item) ? item : null
  )
  return series.some((item) => item !== null) ? series : null
}

export function lastNumber(series: readonly (number | null)[]): number | null {
  return series.findLast((item) => item !== null) ?? null
}

const unitRanges = new Map([
  ["percent", { min: 0, max: 100 }],
  ["percentunit", { min: 0, max: 1 }],
])

export function gaugeRange(
  config: Pick<FieldConfig, "unit" | "min" | "max">,
  data: { fieldMin?: number; fieldMax?: number }
): { min: number; max: number } {
  const implied = unitRanges.get(config.unit ?? "")
  const min = config.min ?? implied?.min ?? Math.min(0, data.fieldMin ?? 0)
  const configured = config.max ?? implied?.max ?? data.fieldMax ?? min
  return { min, max: configured <= min ? min + 1 : configured }
}

export function gaugeFraction(
  value: number | null,
  range: { min: number; max: number }
): number {
  if (value === null) return 0
  return Math.min(1, Math.max(0, (value - range.min) / (range.max - range.min)))
}
