import { decimalsByUnit } from "@/lib/units"
import type { Reducer } from "@/spec"

import {
  dataRange,
  displayValue,
  fieldRange,
  gaugeFraction,
  gaugeRange,
  type FieldConfig,
  type FieldResolver,
  type LinkScope,
} from "../field-config"
import { reduce } from "../reducers"
import { reduceItems, reduceOptionsOf } from "../reduce-items"
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

export type NamePlacement = "above" | "below" | "left" | "hidden"

export type BarLimits = {
  minWidth?: number
  minHeight?: number
  maxHeight?: number
}

export type BarSizes = { titleSize?: number; valueSize?: number }

export type BarStyle = "basic" | "lcd"

export type ValueMode = "color" | "text" | "hidden"

const lcdCellSize = 10
const lcdMinCells = 4
const lcdMaxCells = 40

export function reducerOf(display: Record<string, unknown>): Reducer {
  return display.reduce as Reducer
}

export function orientationOf(
  display: Record<string, unknown>,
  size: { width: number; height: number }
): Orientation {
  if (display.orientation === "horizontal") return "horizontal"
  if (display.orientation === "vertical") return "vertical"
  return size.width > size.height ? "horizontal" : "vertical"
}

export function namePlacementOf(
  display: Record<string, unknown>,
  orientation: Orientation
): NamePlacement {
  const placement = display.namePlacement
  if (placement === "hidden") return "hidden"
  if (orientation === "horizontal") {
    return placement === "left" ? "left" : "above"
  }
  return placement === "top" ? "above" : "below"
}

export function barStyleOf(display: Record<string, unknown>): BarStyle {
  return display.barStyle === "lcd" ? "lcd" : "basic"
}

export function valueModeOf(display: Record<string, unknown>): ValueMode {
  if (display.valueMode === "hidden") return "hidden"
  return display.valueMode === "text" ? "text" : "color"
}

export function showUnfilledOf(display: Record<string, unknown>): boolean {
  return display.showUnfilled !== false
}

function positive(value: unknown): number | undefined {
  return typeof value === "number" && value > 0 ? value : undefined
}

export function barLimits(display: Record<string, unknown>): BarLimits {
  return {
    minWidth: positive(display.minWidth),
    minHeight: positive(display.minHeight),
    maxHeight: positive(display.maxHeight),
  }
}

export function barSizes(display: Record<string, unknown>): BarSizes {
  return {
    titleSize: positive(display.titleSize),
    valueSize: positive(display.valueSize),
  }
}

export function lcdCellCount(extent: number): number {
  const fitting = Math.floor(extent / lcdCellSize)
  return Math.min(lcdMaxCells, Math.max(lcdMinCells, fitting))
}

export function lcdLitCells(fraction: number, cells: number): number {
  if (!(fraction > 0)) return 0
  return Math.min(cells, Math.max(1, Math.round(fraction * cells)))
}

export function bargaugeModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  zone: string | undefined
): BargaugeModel {
  const entries = reduceItems(
    frames,
    fields,
    reduceOptionsOf("bargauge", display),
    zone
  )
  const shared = dataRange(new Set(entries.map((entry) => entry.field)))
  const decimals = decimalsByUnit(
    entries.map(({ config, value }) => ({
      unit: config.unit,
      decimals: config.decimals,
      value,
    }))
  )
  const bars = entries.map(
    ({ key, label, frame, field, config, value, raw, row }): Bar => {
      const range = gaugeRange(
        config,
        config.fieldMinMax ? fieldRange(field) : shared
      )
      const { min, max } = range
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
        fraction: gaugeFraction(shown.numeric, range),
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

export function barScope(
  fields: FieldResolver,
  bar: Bar,
  reducer: Reducer,
  scope: LinkScope
): LinkScope {
  return {
    ...scope,
    row: rowOf(bar, reducer),
    labels: bar.frame.labels,
    seriesName: fields.seriesName(bar.frame, bar.field),
    fieldName: bar.field.name,
    value: bar.value,
    valueText: bar.text,
  }
}
