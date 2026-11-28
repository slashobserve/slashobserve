import type { WireField } from "@/panels/frames"

import { fieldRange } from "./display"
import type { FieldConfig } from "./resolve"

type GaugeRange = { min: number; max: number }

const unitRanges = new Map([
  ["percent", { min: 0, max: 100 }],
  ["percentunit", { min: 0, max: 1 }],
])

export function gaugeRange(
  config: Pick<FieldConfig, "unit" | "min" | "max">,
  data: { min?: number; max?: number }
): GaugeRange {
  const implied = unitRanges.get(config.unit ?? "")
  const min = config.min ?? implied?.min ?? Math.min(0, data.min ?? 0)
  const max = config.max ?? implied?.max ?? data.max ?? min
  return { min, max: max <= min ? min + 1 : max }
}

export function gaugeFraction(value: number | null, range: GaugeRange): number {
  if (value === null) return 0
  return Math.min(1, Math.max(0, (value - range.min) / (range.max - range.min)))
}

export function dataRange(fields: Iterable<WireField>): {
  min?: number
  max?: number
} {
  let min: number | undefined
  let max: number | undefined
  for (const field of fields) {
    const range = fieldRange(field)
    if (range.min !== undefined && (min === undefined || range.min < min)) {
      min = range.min
    }
    if (range.max !== undefined && (max === undefined || range.max > max)) {
      max = range.max
    }
  }
  return {
    ...(min === undefined ? {} : { min }),
    ...(max === undefined ? {} : { max }),
  }
}
