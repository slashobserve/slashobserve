import { styleForSlot } from "../colors"
import { displayValue, type FieldResolver } from "../field-config"
import { seriesKey, uniqueKey } from "../frames"
import { isNumber } from "../reducers"
import type { PreparedField, PreparedFrame } from "../transforms"

export type TimeRange = { from: number; to: number }

export type Segment = {
  from: number
  to: number
  text: string
  color: string
}

export type TimelineRow = { key: string; name: string; segments: Segment[] }

export type TimelineState = { text: string; color: string }

export type TimelineModel = { rows: TimelineRow[]; states: TimelineState[] }

type Sample = { at: number; text: string | null; color: string | null }

type RawSegment = Omit<Segment, "color"> & { color: string | null }

export function mergeValuesOf(display: Record<string, unknown>): boolean {
  return display.mergeValues !== false
}

function isShown(field: PreparedField): boolean {
  return (
    field.type === "number" || field.type === "string" || field.type === "bool"
  )
}

function samplesOf(
  frame: PreparedFrame,
  field: PreparedField,
  fields: FieldResolver
): Sample[] | null {
  const time = frame.fields.find((candidate) => candidate.type === "time")
  if (!time) return null
  const config = fields.configFor(frame, field)
  const samples: Sample[] = []
  time.values.forEach((at, index) => {
    if (!isNumber(at)) return
    const value = field.values[index]
    if (value === null || value === undefined) {
      samples.push({ at, text: null, color: null })
      return
    }
    const shown = displayValue(value, config)
    samples.push({ at, text: shown.text, color: shown.color })
  })
  return samples.sort((a, b) => a.at - b.at)
}

function segmentsOf(
  samples: Sample[],
  range: TimeRange,
  merge: boolean
): RawSegment[] {
  const segments: RawSegment[] = []
  let open: RawSegment | null = null
  samples.forEach((sample, index) => {
    const from = Math.max(sample.at, range.from)
    const to = Math.min(samples[index + 1]?.at ?? range.to, range.to)
    if (sample.text === null || to <= from) {
      open = null
      return
    }
    if (merge && open && open.text === sample.text && open.to === from) {
      open.to = to
      return
    }
    open = { from, to, text: sample.text, color: sample.color }
    segments.push(open)
  })
  return segments
}

export function timelineModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Record<string, unknown>,
  range: TimeRange
): TimelineModel {
  const merge = mergeValuesOf(display)
  const seen = new Map<string, number>()
  const raw: { key: string; name: string; segments: RawSegment[] }[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (!isShown(field) || fields.configFor(frame, field).hidden) continue
      const samples = samplesOf(frame, field, fields)
      if (!samples) continue
      raw.push({
        key: uniqueKey(seen, seriesKey(frame, field.name)),
        name: fields.displayName(frame, field),
        segments: segmentsOf(samples, range, merge),
      })
    }
  }
  const slots = new Map<string, number>()
  for (const { segments } of raw) {
    for (const { text } of segments) {
      if (!slots.has(text)) slots.set(text, slots.size)
    }
  }
  const palette = (text: string) => styleForSlot(slots.get(text) ?? 0).color
  const states = new Map<string, TimelineState>()
  const rows = raw.map(({ key, name, segments }) => ({
    key,
    name,
    segments: segments.map((segment) => {
      const color = segment.color ?? palette(segment.text)
      if (!states.has(segment.text)) {
        states.set(segment.text, { text: segment.text, color })
      }
      return { ...segment, color }
    }),
  }))
  return { rows, states: [...states.values()] }
}

export function segmentAt(segments: Segment[], at: number): Segment | null {
  let low = 0
  let high = segments.length - 1
  while (low <= high) {
    const middle = (low + high) >> 1
    const segment = segments[middle]
    if (at < segment.from) high = middle - 1
    else if (at >= segment.to) low = middle + 1
    else return segment
  }
  return null
}
