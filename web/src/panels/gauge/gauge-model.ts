import { decimalsByUnit } from "@/lib/units"
import type { Reducer } from "@/spec"

import {
  displayValue,
  fieldRange,
  isDataLink,
  resolveLink,
  type FieldConfig,
  type FieldResolver,
  type LinkScope,
  type ResolvedLink,
} from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { reduce } from "../reducers"
import {
  thresholdColor,
  thresholdFor,
  type ThresholdState,
} from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

export type GaugeMarker = { from: number; to: number; color: string }

export type Gauge = {
  key: string
  name: string
  value: number | null
  meterValue: number
  text: string
  color: string | null
  threshold: ThresholdState
  fraction: number
  min: number
  max: number
  markers: GaugeMarker[]
  config: FieldConfig
  frame: PreparedFrame
  field: PreparedField
}

type Source = {
  frame: PreparedFrame
  field: PreparedField
  config: FieldConfig
}

const center = 50
const startAngle = 150
const sweepAngle = 240
const valueCharWidth = 0.62
const valueFit = 43
const moreLinksWidth = 1.75

const unitRanges = new Map([
  ["percent", { min: 0, max: 100 }],
  ["percentunit", { min: 0, max: 1 }],
])

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

function point(angle: number, radius: number): string {
  const radians = (angle * Math.PI) / 180
  const x = center + radius * Math.cos(radians)
  const y = center + radius * Math.sin(radians)
  return `${Number(x.toFixed(3))} ${Number(y.toFixed(3))}`
}

export function arcPath(
  fromFraction: number,
  toFraction: number,
  radius: number
): string {
  const from = clamp(fromFraction, 0, 1)
  const to = clamp(toFraction, 0, 1)
  if (to <= from) return ""
  const largeArc = (to - from) * sweepAngle > 180 ? 1 : 0
  return `M ${point(startAngle + from * sweepAngle, radius)} A ${radius} ${radius} 0 ${largeArc} 1 ${point(startAngle + to * sweepAngle, radius)}`
}

export function valueFontSize(text: string, withMoreLinks = false): number {
  const textWidth = Math.max(text.length, 3) * valueCharWidth
  return valueFit / (textWidth + (withMoreLinks ? moreLinksWidth : 0))
}

function visibleNumberFields(
  frames: PreparedFrame[],
  fields: FieldResolver
): Source[] {
  return frames.flatMap((frame) =>
    frame.fields.flatMap((field) => {
      if (field.type !== "number") return []
      const config = fields.configFor(frame, field)
      return config.hidden ? [] : [{ frame, field, config }]
    })
  )
}

function dataRange(sources: Source[]): { min?: number; max?: number } {
  let min: number | undefined
  let max: number | undefined
  for (const { field } of sources) {
    const range = fieldRange(field)
    if (range.min !== undefined && (min === undefined || range.min < min)) {
      min = range.min
    }
    if (range.max !== undefined && (max === undefined || range.max > max)) {
      max = range.max
    }
  }
  return { min, max }
}

function rangeFor(
  config: FieldConfig,
  data: { min?: number; max?: number }
): { min: number; max: number } {
  const unitRange = unitRanges.get(config.unit ?? "")
  const min = config.min ?? unitRange?.min ?? Math.min(0, data.min ?? 0)
  const max = config.max ?? unitRange?.max ?? data.max ?? min
  return { min, max: max <= min ? min + 1 : max }
}

function markersFor(
  config: FieldConfig,
  min: number,
  max: number
): GaugeMarker[] {
  const steps = config.thresholds ?? []
  const percentage = config.thresholdMode === "percentage"
  const fractionOf = (value: number | undefined): number => {
    if (value === undefined) return 0
    return clamp(percentage ? value / 100 : (value - min) / (max - min), 0, 1)
  }
  return steps.flatMap((step, index) => {
    const from = fractionOf(step.value)
    const next = steps[index + 1]
    const to = next ? fractionOf(next.value) : 1
    return from < to ? [{ from, to, color: thresholdColor(step.color) }] : []
  })
}

export function gaugeModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): Gauge[] {
  const sources = visibleNumberFields(frames, fields)
  const reducer = display.reduce as Reducer
  const withMarkers = display.thresholdMarkers === true
  const data = dataRange(sources)
  const values = sources.map(({ field }) => reduce(field.values, reducer))
  const decimals = decimalsByUnit(
    sources.map(({ config }, index) => ({
      unit: config.unit,
      decimals: config.decimals,
      value: values[index],
    }))
  )
  const seen = new Map<string, number>()
  return sources.map(({ frame, field, config }, index) => {
    const value = values[index]
    const { min, max } = rangeFor(config, data)
    const shown = displayValue(value, config, {
      decimals: decimals.get(config.unit),
      fieldMin: min,
      fieldMax: max,
    })
    const thresholdColored = shown.threshold.color === shown.color
    return {
      key: uniqueKey(seen, seriesKey(frame, field.name)),
      name: fields.displayName(frame, field),
      value,
      meterValue: clamp(value ?? min, min, max),
      text: shown.text,
      color: shown.color,
      threshold: thresholdColored ? shown.threshold : thresholdFor(null),
      fraction: value === null ? 0 : clamp((value - min) / (max - min), 0, 1),
      min,
      max,
      markers: withMarkers ? markersFor(config, min, max) : [],
      config,
      frame,
      field,
    }
  })
}

export function gaugeLinks(
  gauge: Gauge,
  fields: FieldResolver,
  scope: Omit<
    LinkScope,
    "row" | "seriesName" | "fieldName" | "value" | "valueText"
  >
): ResolvedLink[] {
  const { frame, field } = gauge
  const full: LinkScope = {
    ...scope,
    row: { frame, index: field.values.length - 1 },
    labels: frame.labels,
    seriesName: fields.seriesName(frame, field),
    fieldName: field.name,
    value: gauge.value,
    valueText: gauge.text,
  }
  return gauge.config.links
    .filter(isDataLink)
    .flatMap((link) => resolveLink(link, full) ?? [])
}
