import { formatDateTime } from "@/lib/time"

import { displayValue, type FieldResolver } from "../field-config"
import { uniqueKey } from "../frames"
import type { PreparedField, PreparedFrame } from "../transforms"

export const maxEntries = 1000

export type LogsDisplay = {
  wrap: boolean
  showTime: boolean
  order: "newest" | "oldest"
}

export type LogLevelKind = "error" | "warning" | "info" | "other"

export type LogLevel = { word: string; kind: LogLevelKind }

export type LogDetail = {
  name: string
  field: PreparedField
  frame: PreparedFrame
  index: number
}

export type LogEntry = {
  key: string
  time: number | null
  level: LogLevel | null
  message: string
  details: LogDetail[]
}

export type LogsModel = { entries: LogEntry[]; truncated: boolean }

const messageNames = new Set(["body", "message", "msg", "line", "log"])
const levelNames = new Set([
  "severitytext",
  "severity",
  "level",
  "lvl",
  "detected_level",
])

const levelKinds = new Map<string, LogLevelKind>([
  ["error", "error"],
  ["err", "error"],
  ["fatal", "error"],
  ["critical", "error"],
  ["crit", "error"],
  ["alert", "error"],
  ["emerg", "error"],
  ["panic", "error"],
  ["warn", "warning"],
  ["warning", "warning"],
  ["info", "info"],
  ["information", "info"],
  ["notice", "info"],
])

const maxEpochMs = 8.64e15

type Columns = {
  time: PreparedField | undefined
  message: PreparedField | undefined
  level: PreparedField | undefined
}

type Located = {
  frame: PreparedFrame
  columns: Columns
  row: number
  time: number | null
}

function named(
  frame: PreparedFrame,
  names: Set<string>
): PreparedField | undefined {
  return frame.fields.find((field) => names.has(field.name.toLowerCase()))
}

function columnsOf(frame: PreparedFrame): Columns {
  const level = named(frame, levelNames)
  const message =
    named(frame, messageNames) ??
    frame.fields.find((field) => field.type === "string" && field !== level) ??
    frame.fields.find((field) => field.type !== "time" && field !== level)
  return {
    time: frame.fields.find((field) => field.type === "time"),
    message,
    level,
  }
}

function timeOf(value: unknown): number | null {
  return typeof value === "number" && Math.abs(value) <= maxEpochMs
    ? value
    : null
}

function capitalise(word: string): string {
  return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase()
}

export function logLevel(value: unknown): LogLevel | null {
  if (value === null || value === undefined) return null
  const text = String(value).trim()
  if (text === "") return null
  const lower = text.toLowerCase()
  const kind = levelKinds.get(lower) ?? "other"
  return { word: lower === "err" ? "Error" : capitalise(text), kind }
}

function messageOf(
  frame: PreparedFrame,
  field: PreparedField | undefined,
  row: number,
  fields: FieldResolver
): string {
  if (!field) return ""
  const value = field.values[row]
  if (field.type === "string") return typeof value === "string" ? value : ""
  return displayValue(value, fields.configFor(frame, field)).text
}

export function detailText(fields: FieldResolver, detail: LogDetail): string {
  const value = detail.field.values[detail.index]
  const shown = displayValue(
    value,
    fields.configFor(detail.frame, detail.field)
  )
  const time = timeOf(value)
  return detail.field.type === "time" && time !== null && !shown.mapped
    ? formatDateTime(time)
    : shown.text
}

function rowsOf(frame: PreparedFrame): number {
  let count = 0
  for (const field of frame.fields) {
    if (field.values.length > count) count = field.values.length
  }
  return count
}

function compareTimes(a: number | null, b: number | null, factor: number) {
  if (a === null) return b === null ? 0 : 1
  if (b === null) return -1
  return (a - b) * factor
}

export function logsModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Pick<LogsDisplay, "order">
): LogsModel {
  const located: Located[] = []
  for (const frame of frames) {
    const columns = columnsOf(frame)
    const count = rowsOf(frame)
    for (let row = 0; row < count; row++) {
      located.push({
        frame,
        columns,
        row,
        time: timeOf(columns.time?.values[row]),
      })
    }
  }
  const factor = display.order === "oldest" ? 1 : -1
  located.sort((a, b) => compareTimes(a.time, b.time, factor))

  const seen = new Map<string, number>()
  const entries = located
    .slice(0, maxEntries)
    .map(({ frame, columns, row, time }): LogEntry => {
      const message = messageOf(frame, columns.message, row, fields)
      return {
        key: uniqueKey(
          seen,
          `${frame.ref}|${time ?? ""}|${message.slice(0, 120)}`
        ),
        time,
        level: logLevel(columns.level?.values[row]),
        message,
        details: frame.fields.map((field) => ({
          name: field.name,
          field,
          frame,
          index: row,
        })),
      }
    })
  return { entries, truncated: located.length > maxEntries }
}
