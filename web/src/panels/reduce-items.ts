import { formatDateTime } from "@/lib/time"
import {
  compileNamePattern,
  displayWithDefaults,
  type DataPanelType,
  type Reducer,
} from "@/spec"

import type { FieldConfig, FieldResolver } from "./field-config"
import { seriesKey, uniqueKey } from "./frames"
import { isNumber, reduce } from "./reducers"
import {
  matchesName,
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "./transforms"

export type ReduceType = Extract<
  DataPanelType,
  "stat" | "gauge" | "bargauge" | "piechart"
>

export type ReduceOptions = {
  reduce: Reducer
  allValues: boolean
  limit: number
  fields?: string
}

type ReducedValue = number | string | boolean | null

export type ReduceItem = {
  key: string
  label: string
  value: number | null
  raw: ReducedValue
  config: FieldConfig
  frame: PreparedFrame
  field: PreparedField
  row?: number
}

const pickedFromStart = new Set<Reducer>(["first", "firstNotNull"])
const pickedFromEnd = new Set<Reducer>(["last", "lastNotNull"])

export function reduceOptionsOf(
  type: ReduceType,
  display: unknown
): ReduceOptions {
  const merged = displayWithDefaults(type, display)
  return {
    reduce: merged.reduce as Reducer,
    allValues: merged.allValues === true,
    limit: merged.limit as number,
    fields:
      typeof merged.fields === "string" && merged.fields !== ""
        ? merged.fields
        : undefined,
  }
}

function selectorFor(
  fields: string | undefined
): (frame: PreparedFrame, field: PreparedField) => boolean {
  if (fields === undefined) return (_frame, field) => field.type === "number"
  if (!fields.startsWith("/")) {
    return (frame, field) =>
      field.type !== "time" && matchesName(frame, field, fields)
  }
  const pattern = compileNamePattern(fields)
  if (!pattern) return () => false
  return (frame, field) =>
    field.type !== "time" &&
    (pattern.test(field.name) || pattern.test(seriesName(frame, field)))
}

function plain(value: unknown): ReducedValue {
  if (typeof value === "string" || typeof value === "boolean") return value
  return isNumber(value) ? value : null
}

function reducedRaw(field: PreparedField, reducer: Reducer): ReducedValue {
  const value = reduce(field.values, reducer)
  if (field.type === "number" || value !== null) return value
  const present = field.values.filter(
    (item) => item !== null && item !== undefined
  )
  if (pickedFromStart.has(reducer)) return plain(present[0])
  if (pickedFromEnd.has(reducer)) return plain(present[present.length - 1])
  return null
}

function rowLabel(frame: PreparedFrame, row: number): string {
  for (const field of frame.fields) {
    const value = field.values[row]
    if (field.type === "string" && typeof value === "string" && value !== "") {
      return value
    }
  }
  const time = frame.fields.find((field) => field.type === "time")
  const at = time?.values[row]
  return typeof at === "number" ? formatDateTime(at) : String(row + 1)
}

function numeric(raw: ReducedValue): number | null {
  return typeof raw === "number" ? raw : null
}

export function reduceItems(
  frames: PreparedFrame[],
  resolver: FieldResolver,
  options: ReduceOptions
): ReduceItem[] {
  const selected = selectorFor(options.fields)
  const seen = new Map<string, number>()
  const items: ReduceItem[] = []
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (!selected(frame, field)) continue
      const config = resolver.configFor(frame, field)
      if (config.hidden) continue
      const base = seriesKey(frame, field.name)
      if (!options.allValues) {
        const raw = reducedRaw(field, options.reduce)
        items.push({
          key: uniqueKey(seen, base),
          label: resolver.displayName(frame, field),
          value: numeric(raw),
          raw,
          config,
          frame,
          field,
        })
        continue
      }
      const rows = Math.min(field.values.length, options.limit)
      for (let row = 0; row < rows; row++) {
        const raw = plain(field.values[row])
        items.push({
          key: uniqueKey(seen, `${base}@${row}`),
          label: rowLabel(frame, row),
          value: numeric(raw),
          raw,
          config,
          frame,
          field,
          row,
        })
      }
    }
  }
  return items
}
