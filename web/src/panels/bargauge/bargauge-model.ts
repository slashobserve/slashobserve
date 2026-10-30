import { decimalsByUnit } from "@/lib/units"
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
import { reduce } from "../reducers"
import { reduceItems, reduceOptionsOf, type ReduceItem } from "../reduce-items"
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
  row?: number
}

export type BargaugeModel = { bars: Bar[] }

type Entry = ReduceItem

export function reducerOf(display: Record<string, unknown>): Reducer {
  return display.reduce as Reducer
}

const unitRanges = new Map([
  ["percent", { min: 0, max: 100 }],
  ["percentunit", { min: 0, max: 1 }],
])

export function orientationOf(
  display: Record<string, unknown>,
  size: { width: number; height: number }
): Orientation {
  if (display.orientation === "horizontal") return "horizontal"
  if (display.orientation === "vertical") return "vertical"
  return size.width > size.height ? "horizontal" : "vertical"
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

function clamp(fraction: number): number {
  return Math.min(1, Math.max(0, fraction))
}

export function bargaugeModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): BargaugeModel {
  const entries = reduceItems(
    frames,
    fields,
    reduceOptionsOf("bargauge", display)
  )
  const shared = sharedRange(entries)
  const decimals = decimalsByUnit(
    entries.map(({ config, value }) => ({
      unit: config.unit,
      decimals: config.decimals,
      value,
    }))
  )
  const bars = entries.map(
    ({ key, label, frame, field, config, value, raw, row }): Bar => {
      const implied = unitRanges.get(config.unit ?? "")
      const min = config.min ?? implied?.min ?? shared.low
      const configured = config.max ?? implied?.max ?? shared.high
      const max = configured <= min ? min + 1 : configured
      const shown = displayValue(raw, config, {
        decimals: decimals.get(config.unit),
        fieldMin: min,
        fieldMax: max,
      })
      const { threshold } = shown
      const own = threshold.level !== "base" && threshold.color === shown.color
      return {
        key,
        name: label,
        value,
        text: shown.text,
        color: shown.color,
        threshold: own ? threshold.level : "base",
        word: own ? threshold.label : "",
        min,
        max,
        fraction:
          shown.numeric === null
            ? 0
            : clamp((shown.numeric - min) / (max - min)),
        frame,
        field,
        config,
        row,
      }
    }
  )
  return { bars }
}

function lastPresent(values: readonly unknown[]): unknown {
  return values.findLast((value) => value !== null && value !== undefined)
}

function rowOf(bar: Bar, reducer: Reducer): LinkScope["row"] {
  const { frame } = bar
  if (bar.row !== undefined) return { frame, index: bar.row }
  const fields = frame.fields.map((field) => ({
    ...field,
    values: [
      field.type === "number"
        ? reduce(field.values, reducer)
        : (lastPresent(field.values) ?? null),
    ],
  }))
  return { frame: { ...frame, fields }, index: 0 }
}

export function valueTextOf(bar: Bar): string {
  return bar.word ? `${bar.text}, ${bar.word}` : bar.text
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
