import { commonDecimals } from "@/lib/units"
import type { Reducer } from "@/spec"

import {
  displayValue,
  isDataLink,
  resolveLink,
  type FieldConfig,
  type FieldResolver,
  type LinkScope,
  type ResolvedLink,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { reduce } from "../reducers"
import type { ThresholdLevel } from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

export type Orientation = "horizontal" | "vertical"

export type Bar = {
  key: string
  name: string
  value: number | null
  text: string
  color: string | null
  threshold: ThresholdLevel
  word: string
  min: number
  max: number
  fraction: number
  frame: PreparedFrame
  field: PreparedField
  config: FieldConfig
}

export type BargaugeModel = { orientation: Orientation; bars: Bar[] }

type Entry = {
  key: string
  frame: PreparedFrame
  field: PreparedField
  config: FieldConfig
  value: number | null
}

export function reducerOf(display: Record<string, unknown>): Reducer {
  return display.reduce as Reducer
}

function orientationOf(
  display: Record<string, unknown>,
  size: { width: number; height: number }
): Orientation {
  if (display.orientation === "horizontal") return "horizontal"
  if (display.orientation === "vertical") return "vertical"
  return size.width > size.height ? "horizontal" : "vertical"
}

function entriesOf(
  frames: PreparedFrame[],
  fields: FieldResolver,
  reducer: Reducer
): Entry[] {
  const seen = new Map<string, number>()
  const entries: Entry[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      const config = fields.configFor(frame, field)
      if (config.hidden) continue
      entries.push({
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        frame,
        field,
        config,
        value: reduce(field.values, reducer),
      })
    }
  }
  return entries
}

function sharedRange(entries: Entry[]): { low: number; high: number } {
  let low = 0
  let high = -Infinity
  for (const { value } of entries) {
    if (value === null) continue
    if (value < low) low = value
    if (value > high) high = value
  }
  return { low, high }
}

function decimalsByUnit(entries: Entry[]): Map<string | undefined, number> {
  const groups = new Map<string | undefined, (number | null)[]>()
  for (const { config, value } of entries) {
    const numbers = groups.get(config.unit) ?? []
    groups.set(config.unit, numbers)
    if (config.decimals === undefined) numbers.push(value)
  }
  const decimals = new Map<string | undefined, number>()
  for (const [unit, numbers] of groups) {
    const common = commonDecimals(numbers, unit)
    if (common !== undefined) decimals.set(unit, common)
  }
  return decimals
}

function clamp(fraction: number): number {
  return Math.min(1, Math.max(0, fraction))
}

export function bargaugeModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  size: { width: number; height: number }
): BargaugeModel {
  const entries = entriesOf(frames, fields, reducerOf(display))
  const shared = sharedRange(entries)
  const decimals = decimalsByUnit(entries)
  const bars = entries.map(({ key, frame, field, config, value }): Bar => {
    const min = config.min ?? shared.low
    const configured = config.max ?? shared.high
    const max = configured <= min ? min + 1 : configured
    const shown = displayValue(value, config, {
      decimals: decimals.get(config.unit),
      fieldMin: min,
      fieldMax: max,
    })
    const { threshold } = shown
    const own = threshold.level !== "base" && threshold.color === shown.color
    return {
      key,
      name: fields.displayName(frame, field),
      value,
      text: shown.text,
      color: shown.color,
      threshold: own ? threshold.level : "base",
      word: own ? threshold.label : "",
      min,
      max,
      fraction:
        shown.numeric === null ? 0 : clamp((shown.numeric - min) / (max - min)),
      frame,
      field,
      config,
    }
  })
  return { orientation: orientationOf(display, size), bars }
}

function lastPresent(values: readonly unknown[]): unknown {
  return values.findLast((value) => value !== null && value !== undefined)
}

function rowOf(bar: Bar, reducer: Reducer): LinkScope["row"] {
  const { frame } = bar
  const fields = frame.fields.map((field) => ({
    ...field,
    values: [
      field === bar.field
        ? bar.value
        : field.type === "number"
          ? reduce(field.values, reducer)
          : (lastPresent(field.values) ?? null),
    ],
  }))
  return { frame: { ...frame, fields }, index: 0 }
}

export function barLinks(
  fields: FieldResolver,
  bar: Bar,
  reducer: Reducer,
  scope: LinkScope
): ResolvedLink[] {
  const links = bar.config.links.filter(isDataLink)
  if (links.length === 0) return []
  const full: LinkScope = {
    ...scope,
    row: rowOf(bar, reducer),
    labels: bar.frame.labels,
    seriesName: fields.seriesName(bar.frame, bar.field),
    fieldName: bar.field.name,
    value: bar.value,
    valueText: bar.text,
  }
  return links.flatMap((link) => resolveLink(link, full) ?? [])
}
