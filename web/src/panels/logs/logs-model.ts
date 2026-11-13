import { zonedDateTime, zonedDateTimeMs } from "@/lib/time"

import { displayValue, type FieldResolver } from "../field-config"
import { uniqueKey } from "../frames"
import {
  seriesName,
  type PreparedField,
  type PreparedFrame,
} from "../transforms"
import { rowCount } from "../transforms/names"

export const maxEntries = 1000

export type DedupStrategy = "none" | "exact" | "numbers" | "signature"
export type TimePrecision = "ms" | "ns"

export type LogsDisplay = {
  wrap: boolean
  showTime: boolean
  order: "newest" | "oldest"
  showLabels: boolean
  showCommonLabels: boolean
  details: boolean
  prettify: boolean
  dedup: DedupStrategy
  controls: boolean
  fontSize: "default" | "small"
  highlight: boolean
  detailsMode?: "inline" | "sidebar"
  lineFields?: string[]
  showAttributes?: boolean
  timePrecision?: TimePrecision
}

export type Labels = Record<string, string>

export type LogLevelKind = "error" | "warning" | "info" | "other"

export type LogLevel = { word: string; kind: LogLevelKind }

export type LogDetail =
  | { name: string; field: PreparedField; frame: PreparedFrame; index: number }
  | { name: string; text: string }

export type LogEntry = {
  key: string
  time: number | null
  level: LogLevel | null
  message: string
  labels: Labels
  repeats: number
  details: LogDetail[]
}

export type LogsModel = {
  entries: LogEntry[]
  commonLabels: Labels
  truncated: boolean
}

export const messageNames = new Set(["body", "message", "msg", "line", "log"])
const severityNumberNames = new Set(["severitynumber"])
const levelNames = new Set([
  "severitytext",
  "severity",
  "level",
  "lvl",
  "detected_level",
])

const trace: LogLevel = { word: "Trace", kind: "other" }
const debug: LogLevel = { word: "Debug", kind: "other" }
const info: LogLevel = { word: "Info", kind: "info" }
const warning: LogLevel = { word: "Warning", kind: "warning" }
const error: LogLevel = { word: "Error", kind: "error" }
const critical: LogLevel = { word: "Critical", kind: "error" }

const severityBands = [trace, debug, info, warning, error, critical]

const levelsByAlias = new Map<string, LogLevel>([
  ...["trace"].map((alias): [string, LogLevel] => [alias, trace]),
  ...["dbug", "debug"].map((alias): [string, LogLevel] => [alias, debug]),
  ...["info", "information", "informational", "notice"].map(
    (alias): [string, LogLevel] => [alias, info]
  ),
  ...["warn", "warning"].map((alias): [string, LogLevel] => [alias, warning]),
  ...["err", "eror", "error"].map((alias): [string, LogLevel] => [
    alias,
    error,
  ]),
  ...["emerg", "fatal", "alert", "crit", "critical", "panic"].map(
    (alias): [string, LogLevel] => [alias, critical]
  ),
])

const maxEpochMs = 8.64e15

type Columns = {
  time: PreparedField | undefined
  message: PreparedField | undefined
  level: PreparedField | undefined
  severityNumber: PreparedField | undefined
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
    severityNumber: named(frame, severityNumberNames),
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
  return (
    levelsByAlias.get(text.toLowerCase()) ?? {
      word: capitalise(text),
      kind: "other",
    }
  )
}

function severityLevel(value: unknown): LogLevel | null {
  if (typeof value !== "number" || !Number.isInteger(value)) return null
  return severityBands[Math.floor((value - 1) / 4)] ?? null
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
  const text = displayValue(value, fields.configFor(frame, field)).text
  const series = seriesName(frame, field)
  return series === field.name ? text : `${series} ${text}`
}

function lineText(
  frame: PreparedFrame,
  row: number,
  names: readonly string[],
  fields: FieldResolver,
  zone: string | undefined
): string {
  const parts: string[] = []
  for (const name of names) {
    const field = frame.fields.find((candidate) => candidate.name === name)
    const value = field?.values[row]
    const text = field
      ? value === null || value === undefined
        ? ""
        : detailText(fields, { name, field, frame, index: row }, zone)
      : (frame.labels?.[name] ?? "")
    if (text !== "") parts.push(text)
  }
  return parts.join(" ")
}

export function showsAttributes(
  display: Pick<LogsDisplay, "showLabels" | "showAttributes">
): boolean {
  return display.showLabels && display.showAttributes !== false
}

const nanosPerMs = 1e6

export function formatLogTime(
  time: number,
  precision: TimePrecision | undefined,
  zone: string | undefined
): string {
  if (precision !== "ns") return zonedDateTimeMs(time, zone)
  const whole = Math.floor(time)
  const nanos = Math.min(
    nanosPerMs - 1,
    Math.round((time - whole) * nanosPerMs)
  )
  return `${zonedDateTimeMs(whole, zone)}${String(nanos).padStart(6, "0")}`
}

function seriesDetails(frame: PreparedFrame): LogDetail[] {
  const labels = frame.labels ?? {}
  const details: LogDetail[] = Object.keys(labels)
    .sort()
    .map((name) => ({ name, text: labels[name] }))
  return frame.name === undefined
    ? details
    : [{ name: "__name__", text: frame.name }, ...details]
}

function jsonObjectOf(value: unknown): Record<string, unknown> | null {
  if (typeof value !== "string" || !value.trimStart().startsWith("{")) {
    return null
  }
  try {
    const parsed: unknown = JSON.parse(value)
    return typeof parsed === "object" &&
      parsed !== null &&
      !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null
  } catch {
    return null
  }
}

function expandJsonDetail(detail: LogDetail, value: unknown): LogDetail[] {
  const object = jsonObjectOf(value)
  if (object === null) return [detail]
  return Object.entries(object).map(([key, item]) => ({
    name: `${detail.name}.${key}`,
    text: typeof item === "string" ? item : JSON.stringify(item),
  }))
}

export function detailText(
  fields: FieldResolver,
  detail: LogDetail,
  zone: string | undefined
): string {
  if ("text" in detail) return detail.text
  const value = detail.field.values[detail.index]
  const shown = displayValue(
    value,
    fields.configFor(detail.frame, detail.field)
  )
  const time = timeOf(value)
  return detail.field.type === "time" && time !== null && !shown.mapped
    ? zonedDateTime(time, zone)
    : shown.text
}

function compareTimes(a: number | null, b: number | null, factor: number) {
  if (a === null) return b === null ? 0 : 1
  if (b === null) return -1
  return (a - b) * factor
}

const numberPattern = /\d+(?:\.\d+)?/g
const letterOrDigitPattern = /[\p{L}\p{N}]/gu

export function dedupKey(strategy: DedupStrategy, message: string): string {
  switch (strategy) {
    case "numbers":
      return message.replace(numberPattern, "#")
    case "signature":
      return message.replace(letterOrDigitPattern, "")
    default:
      return message
  }
}

export function prettifyMessage(message: string): string {
  const trimmed = message.trim()
  if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return message
  try {
    return JSON.stringify(JSON.parse(trimmed), null, 2)
  } catch {
    return message
  }
}

export type MessageToken = {
  kind: "text" | "key" | "string" | "number" | "level"
  text: string
}

const tokenPattern =
  /([A-Za-z_][\w.-]*)(=)("(?:[^"\\]|\\.)*"|[^\s"]*)|\b(TRACE|DEBUG|INFO|WARN|WARNING|ERROR|FATAL|CRITICAL|PANIC)\b/g
const plainNumber = /^-?\d+(?:\.\d+)?$/

export function messageTokens(message: string): MessageToken[] {
  const tokens: MessageToken[] = []
  let cursor = 0
  const text = (end: number) => {
    if (end > cursor) {
      tokens.push({ kind: "text", text: message.slice(cursor, end) })
    }
  }
  for (const match of message.matchAll(tokenPattern)) {
    text(match.index)
    cursor = match.index + match[0].length
    if (match[4] !== undefined) {
      tokens.push({ kind: "level", text: match[4] })
      continue
    }
    const value = match[3]
    tokens.push({ kind: "key", text: match[1] }, { kind: "text", text: "=" })
    if (value === "") continue
    tokens.push({
      kind: value.startsWith('"')
        ? "string"
        : plainNumber.test(value)
          ? "number"
          : "text",
      text: value,
    })
  }
  text(message.length)
  return tokens
}

function commonLabelsOf(entries: LogEntry[]): Labels {
  const [first, ...rest] = entries
  if (!first) return {}
  return Object.fromEntries(
    Object.entries(first.labels).filter(([name, value]) =>
      rest.every((entry) => entry.labels[name] === value)
    )
  )
}

function withoutLabels(labels: Labels, common: Labels): Labels {
  return Object.fromEntries(
    Object.entries(labels).filter(([name]) => !(name in common))
  )
}

export function logsModel(
  frames: PreparedFrame[],
  fields: FieldResolver,
  display: Pick<LogsDisplay, "order" | "dedup" | "prettify" | "lineFields">,
  zone: string | undefined
): LogsModel {
  const located: Located[] = []
  for (const frame of frames) {
    const columns = columnsOf(frame)
    const count = rowCount(frame)
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

  const deduplicating = display.dedup !== "none"
  const drafted = deduplicating
    ? deduplicate(
        located.map((item) => entryOf(item, fields, display.lineFields, zone)),
        display.dedup
      )
    : located
        .slice(0, maxEntries)
        .map((item) => entryOf(item, fields, display.lineFields, zone))
  const total = deduplicating ? drafted.length : located.length
  const shown = drafted.slice(0, maxEntries)

  const seen = new Map<string, number>()
  const keyed = shown.map(({ item, message, labels, repeats }): LogEntry => {
    const { frame, columns, row, time } = item
    return {
      key: uniqueKey(
        seen,
        `${frame.ref}|${time ?? ""}|${message.slice(0, 120)}`
      ),
      time,
      level:
        logLevel(columns.level?.values[row]) ??
        severityLevel(columns.severityNumber?.values[row]),
      message: display.prettify ? prettifyMessage(message) : message,
      labels,
      repeats,
      details: [
        ...frame.fields.flatMap((field) =>
          expandJsonDetail(
            { name: field.name, field, frame, index: row },
            field.values[row]
          )
        ),
        ...seriesDetails(frame).flatMap((detail) =>
          "text" in detail ? expandJsonDetail(detail, detail.text) : [detail]
        ),
      ],
    }
  })
  const commonLabels = commonLabelsOf(keyed)
  return {
    entries: keyed.map((entry) => ({
      ...entry,
      labels: withoutLabels(entry.labels, commonLabels),
    })),
    commonLabels,
    truncated: total > maxEntries,
  }
}

type Drafted = {
  item: Located
  message: string
  labels: Labels
  repeats: number
}

function entryOf(
  item: Located,
  fields: FieldResolver,
  lineFields: readonly string[] | undefined,
  zone: string | undefined
): Drafted {
  return {
    item,
    message: lineFields?.length
      ? lineText(item.frame, item.row, lineFields, fields, zone)
      : messageOf(item.frame, item.columns.message, item.row, fields),
    labels: item.frame.labels ?? {},
    repeats: 1,
  }
}

function deduplicate(drafted: Drafted[], strategy: DedupStrategy): Drafted[] {
  const firstOf = new Map<string, Drafted>()
  const distinct: Drafted[] = []
  for (const entry of drafted) {
    const key = dedupKey(strategy, entry.message)
    const first = firstOf.get(key)
    if (first) {
      first.repeats += 1
    } else {
      firstOf.set(key, entry)
      distinct.push(entry)
    }
  }
  return distinct
}
