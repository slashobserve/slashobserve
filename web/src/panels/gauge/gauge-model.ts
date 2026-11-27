import { decimalsByUnit, formatValue, ownDecimals } from "@/lib/units"
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
import { reduceItems, reduceOptionsOf } from "../reduce-items"
import {
  thresholdColor,
  thresholdFor,
  type ThresholdState,
} from "../thresholds"
import type { PreparedField, PreparedFrame } from "../transforms"

type GaugeMarker = { from: number; to: number; color: string }

type ThresholdLabel = { fraction: number; text: string }

export type GaugeLimits = { minWidth?: number; minHeight?: number }

export type GaugeSizes = { titleSize?: number; valueSize?: number }

type GaugeArc = { from: number; to: number }

export type Gauge = {
  key: string
  name: string
  value: number | null
  meterValue: number
  text: string
  compact: string
  color: string | null
  threshold: ThresholdState
  fraction: number
  arc: GaugeArc
  min: number
  max: number
  markers: GaugeMarker[]
  labels: ThresholdLabel[]
  config: FieldConfig
  frame: PreparedFrame
  field: PreparedField
  row?: number
}

const center = 50
const startAngle = 150
const sweepAngle = 240
const valueCharWidth = 0.62
const valueFit = 43
const moreLinksWidth = 1.75
const labelRadius = 29

function clamp(value: number, low: number, high: number): number {
  return Math.min(high, Math.max(low, value))
}

function point(angle: number, radius: number): string {
  const radians = (angle * Math.PI) / 180
  const x = center + radius * Math.cos(radians)
  const y = center + radius * Math.sin(radians)
  return `${Number(x.toFixed(3))} ${Number(y.toFixed(3))}`
}

export function labelPosition(fraction: number): { x: number; y: number } {
  const radians =
    ((startAngle + clamp(fraction, 0, 1) * sweepAngle) * Math.PI) / 180
  return {
    x: Number((center + labelRadius * Math.cos(radians)).toFixed(3)),
    y: Number((center + labelRadius * Math.sin(radians)).toFixed(3)),
  }
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

const compactLength = 6
const compactMaxDecimals = 3

function plainUnit(unit: string | undefined): boolean {
  return unit === undefined || unit === "" || unit === "none"
}

function readsAsZero(text: string): boolean {
  return !/[1-9]/.test(text)
}

function compactValueText(
  value: number | null,
  shown: { text: string; mapped: boolean },
  config: Pick<FieldConfig, "unit" | "decimals">
): string {
  if (shown.text.length <= compactLength) return shown.text
  if (shown.mapped || value === null || !Number.isFinite(value)) {
    return shown.text
  }
  const units = plainUnit(config.unit) ? [config.unit, "short"] : [config.unit]
  const most = Math.min(
    config.decimals ?? compactMaxDecimals,
    compactMaxDecimals
  )
  let shortest = shown.text
  for (const unit of units) {
    for (let decimals = most; decimals >= 0; decimals--) {
      const text = formatValue(value, unit, decimals)
      if (value !== 0 && readsAsZero(text)) continue
      if (text.length <= compactLength) return text
      if (text.length < shortest.length) shortest = text
    }
  }
  return shortest
}

function arcFor(
  config: FieldConfig,
  value: number | null,
  fraction: number,
  min: number,
  max: number
): GaugeArc {
  if (value === null) return { from: 0, to: 0 }
  const origin =
    config.neutral === undefined
      ? 0
      : clamp((config.neutral - min) / (max - min), 0, 1)
  return { from: Math.min(origin, fraction), to: Math.max(origin, fraction) }
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

function labelDecimals(
  value: number,
  unit: string | undefined,
  decimals: number | undefined
): number | undefined {
  const own = ownDecimals(value, unit)
  if (own === undefined) return decimals
  if (decimals === undefined) return own
  return Math.max(decimals, own)
}

function thresholdLabelsFor(
  config: FieldConfig,
  min: number,
  max: number
): ThresholdLabel[] {
  const percentage = config.thresholdMode === "percentage"
  return (config.thresholds ?? []).flatMap((step) => {
    if (step.value === undefined) return []
    const value = percentage
      ? min + (step.value / 100) * (max - min)
      : step.value
    const fraction = (value - min) / (max - min)
    if (fraction < 0 || fraction > 1) return []
    const decimals = labelDecimals(value, config.unit, config.decimals)
    return [{ fraction, text: formatValue(value, config.unit, decimals) }]
  })
}

function positive(value: unknown): number | undefined {
  return typeof value === "number" && value > 0 ? value : undefined
}

export function gaugeLimits(display: Record<string, unknown>): GaugeLimits {
  return {
    minWidth: positive(display.minWidth),
    minHeight: positive(display.minHeight),
  }
}

export function gaugeSizes(display: Record<string, unknown>): GaugeSizes {
  return {
    titleSize: positive(display.titleSize),
    valueSize: positive(display.valueSize),
  }
}

export function gaugeModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  zone: string | undefined
): Gauge[] {
  const sources = reduceItems(
    frames,
    fields,
    reduceOptionsOf("gauge", display),
    zone
  )
  const withMarkers = display.thresholdMarkers === true
  const withLabels = display.thresholdLabels === true
  const data = dataRange(new Set(sources.map((source) => source.field)))
  const decimals = decimalsByUnit(
    sources.map(({ config, value }) => ({
      unit: config.unit,
      decimals: config.decimals,
      value,
    }))
  )
  return sources.map(
    ({ key, label, raw, value, frame, field, config, row }) => {
      const range = gaugeRange(
        config,
        config.fieldMinMax ? fieldRange(field) : data
      )
      const { min, max } = range
      const shown = displayValue(raw, config, {
        decimals: decimals.get(config.unit),
        fieldMin: min,
        fieldMax: max,
      })
      const thresholdColored = shown.threshold.color === shown.color
      const fraction = gaugeFraction(value, range)
      return {
        key,
        name: label,
        value,
        meterValue: clamp(value ?? min, min, max),
        text: shown.text,
        compact: compactValueText(value, shown, config),
        color: shown.color,
        threshold: thresholdColored ? shown.threshold : thresholdFor(null),
        fraction,
        arc: arcFor(config, value, fraction, min, max),
        min,
        max,
        markers: withMarkers ? markersFor(config, min, max) : [],
        labels: withLabels ? thresholdLabelsFor(config, min, max) : [],
        config,
        frame,
        field,
        row,
      }
    }
  )
}

type BaseScope = Omit<
  LinkScope,
  "row" | "seriesName" | "fieldName" | "value" | "valueText"
>

export function gaugeScope(
  gauge: Gauge,
  fields: FieldResolver,
  scope: BaseScope
): LinkScope {
  const { frame, field } = gauge
  return {
    ...scope,
    row: { frame, index: gauge.row ?? field.values.length - 1 },
    labels: frame.labels,
    seriesName: fields.seriesName(frame, field),
    fieldName: field.name,
    value: gauge.value,
    valueText: gauge.text,
  }
}
