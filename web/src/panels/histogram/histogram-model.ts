import { axisDecimals } from "@/lib/units"

import { styleForSlot } from "../colors"
import type { FieldResolver } from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { isNumber } from "../reducers"
import { thresholdColor } from "../thresholds"
import type { PreparedFrame } from "../transforms"

const defaultBucketCount = 20
const maxBuckets = 1000

type HistogramSeries = {
  key: string
  name: string
  color: string
  counts: number[]
}

export type HistogramModel = {
  edges: number[]
  series: HistogramSeries[]
  total: number
  unit: string | undefined
  decimals: number | undefined
}

type Entry = {
  key: string
  name: string
  color: string
  values: number[]
}

type Buckets = { start: number; size: number; count: number; end?: number }

function positive(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
}

function entriesOf(frames: PreparedFrame[], fields: FieldResolver) {
  const seen = new Map<string, number>()
  const entries: Entry[] = []
  let unit: string | undefined
  let decimals: number | undefined
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type !== "number") continue
      const config = fields.configFor(frame, field)
      if (config.hidden) continue
      if (entries.length === 0) ({ unit, decimals } = config)
      entries.push({
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        name: fields.displayName(frame, field),
        color: config.color
          ? thresholdColor(config.color)
          : styleForSlot(entries.length).color,
        values: field.values.filter(isNumber),
      })
    }
  }
  return { entries, unit, decimals }
}

function extent(entries: Entry[]): { min: number; max: number } | null {
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

function alignedBuckets(min: number, max: number, size: number): Buckets {
  let step = size
  for (;;) {
    const start = Math.floor(min / step) * step
    const count = Math.max(1, Math.ceil(clean((max - start) / step)))
    if (count <= maxBuckets) return { start, size: step, count }
    step *= Math.ceil(count / maxBuckets)
  }
}

function equalBuckets(min: number, max: number, count: number): Buckets {
  if (min === max) return { start: min, size: 0, count: 1, end: max }
  return { start: min, size: (max - min) / count, count, end: max }
}

function bucketsOf(
  min: number,
  max: number,
  display: Record<string, unknown>
): Buckets {
  if (positive(display.bucketSize)) {
    return alignedBuckets(min, max, display.bucketSize)
  }
  const count = display.bucketCount
  return equalBuckets(
    min,
    max,
    typeof count === "number" && Number.isInteger(count) && count >= 1
      ? Math.min(count, maxBuckets)
      : defaultBucketCount
  )
}

function clean(value: number): number {
  return Number(value.toPrecision(12))
}

function edgesOf({ start, size, count, end }: Buckets): number[] {
  const edges = Array.from({ length: count + 1 }, (_, i) =>
    clean(start + i * size)
  )
  if (end !== undefined) edges[count] = end
  return edges
}

function bucketIndex(value: number, { start, size, count }: Buckets): number {
  if (size === 0) return 0
  return Math.min(count - 1, Math.max(0, Math.floor((value - start) / size)))
}

export function histogramModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>
): HistogramModel {
  const { entries, unit, decimals } = entriesOf(frames, fields)
  const range = extent(entries)
  if (range === null) {
    const series = entries.map(({ key, name, color }) => ({
      key,
      name,
      color,
      counts: [],
    }))
    return { edges: [], series, total: 0, unit, decimals }
  }
  const buckets = bucketsOf(range.min, range.max, display)
  const edges = edgesOf(buckets)
  let total = 0
  const series = entries.map(({ key, name, color, values }) => {
    const counts = new Array<number>(buckets.count).fill(0)
    for (const value of values) counts[bucketIndex(value, buckets)]++
    total += values.length
    return { key, name, color, counts }
  })
  return {
    edges,
    series,
    total,
    unit,
    decimals: axisDecimals([...new Set(edges)], unit, decimals),
  }
}

export function stackedMax(model: HistogramModel): number {
  let max = 0
  for (let i = 0; i < model.edges.length - 1; i++) {
    max = Math.max(
      max,
      model.series.reduce((sum, { counts }) => sum + counts[i], 0)
    )
  }
  return max
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

export function labelStride(
  edgeCount: number,
  plotWidth: number,
  labelWidth: number
): number {
  if (edgeCount < 2 || plotWidth <= 0) return 1
  const spacing = plotWidth / (edgeCount - 1)
  return Math.max(1, Math.ceil(labelWidth / spacing))
}
