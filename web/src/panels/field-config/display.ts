import { formatValue } from "@/lib/units"
import type { WireField } from "@/panels/frames"
import {
  thresholdColor,
  thresholdFor,
  type ThresholdState,
} from "@/panels/thresholds"

import { rangePosition, schemeColor } from "../color-scheme"
import { matchMapping, matchSpecial } from "./mappings"
import { effectiveColorMode, type FieldConfig } from "./resolve"

export type DisplayValue = {
  text: string
  numeric: number | null
  color: string | null
  threshold: ThresholdState
  mapped: boolean
}

export type DisplayOptions = {
  decimals?: number
  fieldMin?: number
  fieldMax?: number
}

const noThreshold = thresholdFor(null)
const placeholder = "-"

function absentKind(value: unknown): "null" | "nan" | "empty" | null {
  if (value === null || value === undefined) return "null"
  if (typeof value === "number" && Number.isNaN(value)) return "nan"
  if (value === "") return "empty"
  return null
}

function fixedColorOf(config: FieldConfig): string | null {
  const mode = effectiveColorMode(config)
  return config.color && (mode === "fixed" || mode === "shades")
    ? thresholdColor(config.color)
    : null
}

function schemeColorOf(
  value: number | null,
  config: FieldConfig,
  options: DisplayOptions
): string | null {
  if (
    value === null ||
    !Number.isFinite(value) ||
    !config.colorScheme ||
    effectiveColorMode(config) !== "scheme"
  ) {
    return null
  }
  const min = config.min ?? options.fieldMin
  const max = config.max ?? options.fieldMax
  if (min === undefined || max === undefined) return null
  return schemeColor(config.colorScheme, rangePosition(value, min, max))
}

function thresholdStateFor(
  value: number | null,
  config: FieldConfig,
  options: DisplayOptions,
  decimals: number | undefined
): ThresholdState {
  if (value === null || !config.thresholds) return noThreshold
  if (config.thresholdMode === "absolute") {
    return thresholdFor(value, config.thresholds, config.unit, decimals)
  }
  const min = config.min ?? options.fieldMin
  const max = config.max ?? options.fieldMax
  if (min === undefined || max === undefined) return noThreshold
  const percent = max === min ? 100 : ((value - min) / (max - min)) * 100
  return thresholdFor(percent, config.thresholds, "percent")
}

export function displayValue(
  value: unknown,
  config: FieldConfig,
  options: DisplayOptions = {}
): DisplayValue {
  const absent = absentKind(value)
  if (absent !== null) {
    const mapping = matchSpecial(config.mappings, absent)
    return {
      text: mapping?.text ?? config.noValue ?? placeholder,
      numeric: null,
      color: mapping?.color
        ? thresholdColor(mapping.color)
        : fixedColorOf(config),
      threshold: noThreshold,
      mapped: mapping !== null,
    }
  }
  const decimals = config.decimals ?? options.decimals
  const hit = matchMapping(config.mappings, value)
  const number = typeof value === "number" ? value : null
  const threshold = thresholdStateFor(number, config, options, decimals)
  const text =
    hit?.text ??
    (number === null
      ? String(value)
      : formatValue(number, config.unit, decimals))
  const color = hit?.mapping.color
    ? thresholdColor(hit.mapping.color)
    : (schemeColorOf(number, config, options) ??
      threshold.color ??
      fixedColorOf(config))
  return {
    text,
    numeric: number !== null && Number.isFinite(number) ? number : null,
    color,
    threshold,
    mapped: hit !== null,
  }
}

export function fieldRange(field: WireField): { min?: number; max?: number } {
  if (field.type !== "number") return {}
  let min: number | undefined
  let max: number | undefined
  for (const value of field.values) {
    if (typeof value !== "number" || !Number.isFinite(value)) continue
    if (min === undefined || value < min) min = value
    if (max === undefined || value > max) max = value
  }
  return { min, max }
}
