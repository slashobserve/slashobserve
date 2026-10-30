import type { Transform } from "@/spec"

import { parseDate } from "@/lib/date-format"

import {
  matchesName,
  withFields,
  type PreparedField,
  type PreparedFrame,
} from "./names"

type ConvertOptions = Extract<Transform, { type: "convertFieldType" }>
type Conversion = ConvertOptions["conversions"][number]
type Converted = Pick<PreparedField, "type" | "values">

function toNumber(value: unknown): number | null {
  if (value === null || value === undefined || value === "") return null
  const number = Number(value)
  return Number.isFinite(number) ? number : null
}

function toText(value: unknown, joinWith: string): string | null {
  if (value === null || value === undefined) return null
  if (Array.isArray(value)) return value.join(joinWith)
  if (typeof value === "object") return JSON.stringify(value)
  return String(value)
}

function toTime(value: unknown, dateFormat: string | undefined): number | null {
  if (typeof value === "number") return Number.isFinite(value) ? value : null
  if (typeof value !== "string") return null
  const parsed =
    dateFormat === undefined ? Date.parse(value) : parseDate(value, dateFormat)
  return parsed === null || Number.isNaN(parsed) ? null : parsed
}

function toBoolean(value: unknown): boolean | null {
  if (typeof value === "boolean") return value
  if (typeof value === "number") {
    return Number.isFinite(value) ? value !== 0 : null
  }
  if (typeof value !== "string") return null
  const text = value.trim().toLowerCase()
  if (text === "true") return true
  if (text === "false") return false
  return null
}

function convert(
  field: PreparedField,
  { to, dateFormat, joinWith = "," }: Conversion
): Converted {
  switch (to) {
    case "number":
      return { type: "number", values: field.values.map(toNumber) }
    case "string":
      return {
        type: "string",
        values: field.values.map((value) => toText(value, joinWith)),
      }
    case "time":
      return {
        type: "time",
        values: field.values.map((value) => toTime(value, dateFormat)),
      }
    case "boolean":
      return { type: "bool", values: field.values.map(toBoolean) }
    default:
      return { type: "other", values: field.values }
  }
}

export function convertFieldType(
  frames: PreparedFrame[],
  { conversions }: ConvertOptions
): PreparedFrame[] {
  return frames.map((frame) =>
    withFields(
      frame,
      conversions.reduce(
        (fields, conversion) =>
          fields.map((field) =>
            matchesName(frame, field, conversion.field)
              ? { ...field, ...convert(field, conversion) }
              : field
          ),
        frame.fields
      )
    )
  )
}
