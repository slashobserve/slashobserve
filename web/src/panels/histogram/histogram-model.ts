import { axisFormat } from "@/lib/units"
import { displayWithDefaults, type Reducer } from "@/spec"

import { styleForSlot } from "../colors"
import type { FieldConfig, FieldResolver } from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import type { SeriesLegendEntry } from "../legend/use-series-legend"
import { isNumber } from "../reducers"
import { thresholdColor } from "../thresholds"
import { tooltipRows, type TooltipSort } from "../timeseries/tooltip-rows"
import type { PreparedFrame } from "../transforms"
import {
  logTicks,
  scaleOf,
  symlogTicks,
  valueAxisSettings,
  valueRange,
  type ValueAxisSettings,
  type ValueScale,
  linearScale,
} from "../value-axis"

const defaultBucketCount = 20
const maxBuckets = 1000
const overlayOpacity = 0.7
const combinedKey = "combined"
const combinedName = "All series"

type HistogramStack = "none" | "normal" | "percent"
type HistogramLegend = "bottom" | "right" | "hidden"
type HistogramTooltip = "single" | "all" | "hidden"

export type HistogramEntry = {
  key: string
  name: string
  color: string
  values: number[]
  config: FieldConfig
  fillOpacity: number | null
  borderWidth: number
}

type HistogramSeries = {
  key: string
  name: string
  color: string
  counts: number[]
  fillOpacity: number
  borderWidth: number
  hideInTooltip: boolean
}

type HistogramBar = { start: number; end: number }

type HistogramAxis = {
  settings: ValueAxisSettings
  color: string | null
}

export type HistogramModel = {
  edges: number[]
  series: HistogramSeries[]
  bars: HistogramBar[][]
  total: number
  unit: string | undefined
  format: (edge: number) => string
  stack: HistogramStack
  axis: HistogramAxis
  scale: ValueScale
  min: number
  max: number
}

export type HistogramOptions = {
  stack: HistogramStack
  combine: boolean
  legend: HistogramLegend
  legendMode?: "list" | "table"
  legendValues: Reducer[]
  legendSortBy?: "name" | Reducer
  legendSortDesc: boolean
  legendWidth?: number
  tooltip: HistogramTooltip
  tooltipSort: TooltipSort
  tooltipHideZeros: boolean
  tooltipMaxWidth?: number | undefined
  tooltipMaxHeight?: number | undefined
}

type Buckets = { start: number; size: number; count: number; end?: number }

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
}

function numberOf(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined
}

export function histogramOptions(
  display: Record<string, unknown>
): HistogramOptions {
  const merged = displayWithDefaults("histogram", display)
  const sortBy = merged.legendSortBy
  const legendWidth = numberOf(merged.legendWidth)
  return {
    stack:
      merged.stack === "none" || merged.stack === "percent"
        ? merged.stack
        : "normal",
    combine: merged.combine === true,
    legend:
      merged.legend === "right" || merged.legend === "hidden"
        ? merged.legend
        : "bottom",
    ...(merged.legendMode === "list" || merged.legendMode === "table"
      ? { legendMode: merged.legendMode }
      : {}),
    legendValues: (merged.legendValues ?? []) as Reducer[],
    ...(typeof sortBy === "string"
      ? { legendSortBy: sortBy as "name" | Reducer }
      : {}),
    legendSortDesc: merged.legendSortDesc === true,
    ...(legendWidth === undefined ? {} : { legendWidth }),
    tooltip:
      merged.tooltip === "single" || merged.tooltip === "hidden"
        ? merged.tooltip
        : "all",
    tooltipSort:
      merged.tooltipSort === "asc" || merged.tooltipSort === "desc"
        ? merged.tooltipSort
        : "none",
    tooltipHideZeros: merged.tooltipHideZeros === true,
    tooltipMaxWidth: numberOf(merged.tooltipMaxWidth),
    tooltipMaxHeight: numberOf(merged.tooltipMaxHeight),
  }
}

export function histogramEntries(
  frames: PreparedFrame[],
  fields: FieldResolver,
  combine: boolean
): HistogramEntry[] {
  const seen = new Map<string, number>()
  const entries: HistogramEntry[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      const config = fields.configFor(frame, field)
      if (config.hidden) continue
      entries.push({
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        name: fields.displayName(frame, field),
        color: config.color
          ? thresholdColor(config.color)
          : styleForSlot(entries.length).color,
        values: field.values.filter(isNumber),
        config,
        fillOpacity: fields.isSet(frame, field, "fill")
          ? config.fill / 100
          : null,
        borderWidth: fields.isSet(frame, field, "lineWidth")
          ? config.lineWidth
          : 0,
      })
    }
  }
  const [first] = entries
  if (!combine || first === undefined) return entries
  const values: number[] = []
  for (const entry of entries) {
    for (const value of entry.values) values.push(value)
  }
  return [{ ...first, key: combinedKey, name: combinedName, values }]
}

export function legendEntries(
  entries: readonly HistogramEntry[]
): SeriesLegendEntry[] {
  return entries.map(({ key, name, color, values, config }) => ({
    key,
    label: name,
    color,
    values,
    config,
    hideInLegend: config.hideInLegend,
  }))
}

function extent(entries: readonly HistogramEntry[]) {
  let min = Infinity
  let max = -Infinity
  for (const { values } of entries) {
    for (const value of values) {
      if (value < min) min = value
      if (value > max) max = value
    }
  }
  return min <= max ? { min, max } : null
}

function clean(value: number): number {
  return Number(value.toPrecision(12))
}

function equalBuckets(min: number, max: number, count: number): Buckets {
  if (min === max) return { start: min, size: 0, count: 1, end: max }
  return { start: min, size: (max - min) / count, count, end: max }
}

function alignedBuckets(
  min: number,
  max: number,
  size: number,
  offset: number
): Buckets {
  let step = size
  for (;;) {
    const start = Math.floor((min - offset) / step) * step + offset
    const count = Math.max(1, Math.ceil(clean((max - start) / step)))
    if (!Number.isFinite(start) || !Number.isFinite(count)) {
      return equalBuckets(min, max, maxBuckets)
    }
    if (count <= maxBuckets) return { start, size: step, count }
    step *= Math.ceil(count / maxBuckets)
  }
}

export function bucketsOf(
  min: number,
  max: number,
  display: Record<string, unknown>
): Buckets {
  const offset = numberOf(display.bucketOffset) ?? 0
  if (positive(display.bucketSize)) {
    return alignedBuckets(min, max, display.bucketSize, offset)
  }
  const wanted = display.bucketCount
  const count =
    typeof wanted === "number" && Number.isInteger(wanted) && wanted >= 1
      ? Math.min(wanted, maxBuckets)
      : defaultBucketCount
  if (offset !== 0 && max > min) {
    return alignedBuckets(min, max, (max - min) / count, offset)
  }
  return equalBuckets(min, max, count)
}

export function edgesOf({ start, size, count, end }: Buckets): number[] {
  const edges = Array.from({ length: count + 1 }, (_, i) =>
    clean(start + i * size)
  )
  if (end !== undefined) edges[count] = end
  return edges
}

export function bucketIndex(
  value: number,
  { start, size, count }: Buckets
): number {
  if (size === 0) return 0
  return Math.min(count - 1, Math.max(0, Math.floor((value - start) / size)))
}

function barsOf(
  counts: readonly number[][],
  stack: HistogramStack,
  buckets: number,
  groups: readonly string[]
): HistogramBar[][] {
  const bars = counts.map(() => new Array<HistogramBar>(buckets))
  for (let bucket = 0; bucket < buckets; bucket++) {
    const totals = new Map<string, number>()
    for (const [series, row] of counts.entries()) {
      const group = groups[series] ?? ""
      totals.set(group, (totals.get(group) ?? 0) + (row[bucket] ?? 0))
    }
    const running = new Map<string, number>()
    counts.forEach((row, series) => {
      const count = row[bucket] ?? 0
      const target = bars[series]
      if (target === undefined) return
      if (stack === "none") {
        target[bucket] = { start: 0, end: count }
        return
      }
      const group = groups[series] ?? ""
      const total = totals.get(group) ?? 0
      const scaled =
        stack === "percent" ? (total === 0 ? 0 : (count / total) * 100) : count
      const start = running.get(group) ?? 0
      target[bucket] = { start, end: start + scaled }
      running.set(group, start + scaled)
    })
  }
  return bars
}

const defaultAxisConfig = {
  axis: "left",
  axisBorder: false,
  axisColor: "text",
  scale: "linear",
  logBase: 10,
  axisCenteredZero: false,
} as const

function percentSettings(settings: ValueAxisSettings): ValueAxisSettings {
  const out: ValueAxisSettings = {
    ...settings,
    scale: "linear",
    centeredZero: false,
  }
  delete out.softMin
  delete out.softMax
  return out
}

function axisOf(
  first: HistogramEntry | undefined,
  stack: HistogramStack
): HistogramAxis {
  if (!first) {
    return { settings: valueAxisSettings(defaultAxisConfig), color: null }
  }
  const settings = valueAxisSettings(first.config)
  return {
    settings: stack === "percent" ? percentSettings(settings) : settings,
    color: settings.seriesColored ? first.color : null,
  }
}

const noneHidden: ReadonlySet<string> = new Set()

export function histogramModel(
  allEntries: readonly HistogramEntry[],
  display: Record<string, unknown>,
  hidden: ReadonlySet<string> = noneHidden
): HistogramModel {
  const stack = histogramOptions(display).stack
  const entries = allEntries.filter(({ key }) => !hidden.has(key))
  const first = allEntries[0]
  const unit = first?.config.unit
  const configDecimals = first?.config.decimals
  const axis = axisOf(entries[0] ?? first, stack)
  const range = extent(allEntries)
  const empty = {
    edges: [] as number[],
    bars: [] as HistogramBar[][],
    total: 0,
    unit,
    format: axisFormat([], unit, configDecimals),
    stack,
    axis,
    scale: linearScale,
    min: 0,
    max: 1,
  }
  const describe = (entry: HistogramEntry, counts: number[]) => ({
    key: entry.key,
    name: entry.name,
    color: entry.color,
    counts,
    fillOpacity: entry.fillOpacity ?? (stack === "none" ? overlayOpacity : 1),
    borderWidth: entry.borderWidth,
    hideInTooltip: entry.config.hideInTooltip,
  })
  if (range === null) {
    return { ...empty, series: entries.map((entry) => describe(entry, [])) }
  }
  const buckets = bucketsOf(range.min, range.max, display)
  const edges = edgesOf(buckets)
  let total = 0
  const counts = entries.map(({ values }) => {
    const row = new Array<number>(buckets.count).fill(0)
    for (const value of values) {
      const at = bucketIndex(value, buckets)
      row[at] = (row[at] ?? 0) + 1
    }
    total += values.length
    return row
  })
  const bars = barsOf(
    counts,
    stack,
    buckets.count,
    entries.map(({ config }) => config.stackGroup)
  )
  let top = 0
  for (const row of bars) {
    for (const { end } of row) top = Math.max(top, end)
  }
  const settings = axis.settings
  const [min, max] =
    stack === "percent" ? [0, 100] : valueRange([0, top], 1, settings)
  return {
    edges,
    series: entries.map((entry, index) => describe(entry, counts[index] ?? [])),
    bars,
    total,
    unit,
    format: axisFormat([...new Set(edges)], unit, configDecimals),
    stack,
    axis,
    scale: scaleOf(settings),
    min,
    max: max === min ? min + 1 : max,
  }
}

const tickBases = [1, 2, 5]

export function countTicks(max: number, maxTicks: number): number[] {
  const limit = Math.max(1, Math.floor(maxTicks))
  const top = Math.max(1, Math.ceil(max))
  for (let magnitude = 1; ; magnitude *= 10) {
    for (const base of tickBases) {
      const step = base * magnitude
      if (Math.ceil(top / step) <= limit) {
        return Array.from(
          { length: Math.ceil(top / step) + 1 },
          (_, i) => i * step
        )
      }
    }
  }
}

export function valueTicks(
  model: Pick<HistogramModel, "scale" | "min" | "max" | "stack">,
  maxTicks: number
): { min: number; max: number; ticks: number[] } {
  const { kind, logBase, linearThreshold } = model.scale
  if (kind === "log") {
    return {
      min: model.min,
      max: model.max,
      ticks: logTicks(model.min, model.max, logBase, maxTicks),
    }
  }
  if (kind === "symlog") {
    return {
      min: model.min,
      max: model.max,
      ticks: symlogTicks(model.min, model.max, linearThreshold, maxTicks),
    }
  }
  const ticks = countTicks(model.max, maxTicks)
  const below =
    model.min < 0
      ? countTicks(-model.min, maxTicks)
          .slice(1)
          .map((tick) => -tick)
          .reverse()
      : []
  return {
    min: below[0] ?? 0,
    max: ticks.at(-1) ?? model.max,
    ticks: [...below, ...ticks],
  }
}

export type HistogramHover = { index: number; series: string | null }

export function tooltipSeries(
  model: Pick<HistogramModel, "series">,
  hover: HistogramHover,
  options: Pick<
    HistogramOptions,
    "tooltip" | "tooltipSort" | "tooltipHideZeros"
  >
): { rows: { index: number; value: number }[]; total: number } {
  const candidates = model.series.flatMap((series, index) => {
    const value = series.counts[hover.index]
    return value === undefined ||
      series.hideInTooltip ||
      (options.tooltip === "single" && series.key !== hover.series)
      ? []
      : [{ index, value }]
  })
  const { rows } = tooltipRows(
    candidates,
    {
      sort: options.tooltipSort,
      hideZeros: options.tooltipHideZeros,
      cumulative: false,
      stack: [],
    },
    Infinity
  )
  const total = model.series.reduce(
    (sum, { counts }) => sum + (counts[hover.index] ?? 0),
    0
  )
  return { rows, total }
}

export function labelStride(
  edgeCount: number,
  plotWidth: number,
  labelWidth: number
): number {
  if (edgeCount < 2 || plotWidth <= 0) return 1
  const spacing = plotWidth / (edgeCount - 1)
  return Math.max(1, Math.ceil(labelWidth / spacing))
}
