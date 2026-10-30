import type { WireFrame } from "@/dashboards/queries"

import type { FieldProps } from "@/spec"

import { seriesLabel, type WireField } from "../frames"
import { isNumber } from "../reducers"

export type PreparedField = WireField & {
  seriesName?: string
  config?: FieldProps
}
export type PreparedFrame = Omit<WireFrame, "fields"> & {
  fields: PreparedField[]
}

const collator = new Intl.Collator()

export function numberFieldCount(frame: PreparedFrame): number {
  return frame.fields.filter((field) => field.type === "number").length
}

export function seriesName(frame: PreparedFrame, field: PreparedField): string {
  if (field.seriesName) return field.seriesName
  if (field.type !== "number") return field.name
  return seriesLabel(frame, field, undefined, numberFieldCount(frame))
}

export function matchesName(
  frame: PreparedFrame,
  field: PreparedField,
  name: string
): boolean {
  return field.name === name || seriesName(frame, field) === name
}

export function rowCount(frame: PreparedFrame): number {
  let count = 0
  for (const field of frame.fields) {
    if (field.values.length > count) count = field.values.length
  }
  return count
}

export function withFields(
  frame: PreparedFrame,
  fields: PreparedField[]
): PreparedFrame {
  return { ...frame, fields }
}

export function labelKeys(frame: PreparedFrame): string[] {
  return Object.keys(frame.labels ?? {}).sort()
}

export function firstTimeField(
  frame: PreparedFrame
): PreparedField | undefined {
  return frame.fields.find((field) => field.type === "time")
}

export function copyFrame(frame: PreparedFrame): PreparedFrame {
  return withFields(frame, [...frame.fields])
}

export function cell(field: PreparedField, row: number): unknown {
  return field.values[row] ?? null
}

export function lastPresent(values: readonly unknown[]): unknown {
  for (let i = values.length - 1; i >= 0; i--) {
    const value = values[i]
    if (value !== null && value !== undefined) return value
  }
  return null
}

export function compareValues(a: unknown, b: unknown): number {
  if (isNumber(a) && isNumber(b)) return a - b
  if (typeof a === "boolean" && typeof b === "boolean") {
    return Number(a) - Number(b)
  }
  return collator.compare(String(a), String(b))
}
