import type { WireFrame } from "@/dashboards/queries"

import { seriesLabel, type WireField } from "../frames"

export function numberFieldCount(frame: WireFrame): number {
  return frame.fields.filter((field) => field.type === "number").length
}

export function displayName(frame: WireFrame, field: WireField): string {
  if (field.type !== "number") return field.name
  return seriesLabel(frame, field, undefined, numberFieldCount(frame))
}

export function matchesName(
  frame: WireFrame,
  field: WireField,
  name: string
): boolean {
  return field.name === name || displayName(frame, field) === name
}

export function rowCount(frame: WireFrame): number {
  return frame.fields.reduce(
    (count, field) => Math.max(count, field.values.length),
    0
  )
}

export function withFields(frame: WireFrame, fields: WireField[]): WireFrame {
  return { ...frame, fields }
}

export function copyFrame(frame: WireFrame): WireFrame {
  return withFields(frame, [...frame.fields])
}

export function cell(field: WireField, row: number): unknown {
  return field.values[row] ?? null
}

export function isNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value)
}

export function lastPresent(values: readonly unknown[]): unknown {
  for (let i = values.length - 1; i >= 0; i--) {
    const value = values[i]
    if (value !== null && value !== undefined) return value
  }
  return null
}
