import { displayValue, type FieldConfig } from "../field-config"
import type { XSource } from "../timeseries/align"
import type { XAxis } from "../timeseries/plot-model"
import {
  matchesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"

export const tableFallbackNote =
  "No numeric field besides the x field to plot, so this shows a table. Use Time series for values over time."

export const tableFallbackNoteHeight = 48

export const noXFieldReason = "A trend needs a numeric field for its x axis."

export function xFieldOf(
  frame: PreparedFrame,
  xField: string | undefined
): PreparedField | undefined {
  const numbers = frame.fields.filter((field) => field.type === "number")
  if (xField === undefined) return numbers[0]
  return numbers.find((field) => matchesName(frame, field, xField))
}

export function xFieldName(display: Record<string, unknown>) {
  return typeof display.xField === "string" && display.xField !== ""
    ? display.xField
    : undefined
}

export function trendSource(xField: string | undefined): XSource {
  return { fieldOf: (frame) => xFieldOf(frame, xField), divisor: 1 }
}

export function framesWithX(
  frames: readonly PreparedFrame[],
  source: XSource
): PreparedFrame[] {
  return frames.filter((frame) => source.fieldOf(frame) !== undefined)
}

export function xAxisFor(configJson: string | null): XAxis | null {
  if (configJson === null) return null
  const config = JSON.parse(configJson) as FieldConfig
  return {
    kind: "number",
    format: (value) => displayValue(value, config).text,
  }
}
