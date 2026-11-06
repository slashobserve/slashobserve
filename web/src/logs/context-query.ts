import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { exploreKeys, type ContextParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { WireFrame } from "@/dashboards/queries"
import { matcherInsert } from "@/panel-editor/insert-text"
import type { Labels, LogEntry } from "@/panels/logs/logs-model"

import { keepRows } from "./frames"
import { isSearchFamily } from "./search"
import { detailValue } from "./trace-link"

export const clickhouseLogsTemplate = `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp)
ORDER BY Timestamp DESC
LIMIT 1000`

const perLineLokiLabels = new Set([
  "trace_id",
  "span_id",
  "traceid",
  "spanid",
  "trace_flags",
  "flags",
  "severity_text",
  "severity_number",
  "detected_level",
  "observed_timestamp",
])

export function lokiStreamSelector(
  labels: Labels,
  indexed: readonly string[] | undefined
): string | null {
  const keep =
    indexed !== undefined && indexed.length > 0
      ? (name: string) => indexed.includes(name)
      : (name: string) => !perLineLokiLabels.has(name)
  const names = Object.keys(labels).filter(keep).sort()
  if (names.length === 0) return null
  return `{${names.map((name) => matcherInsert("loki", name, labels[name])).join(", ")}}`
}

export function serviceNameOf(entry: LogEntry): string | null {
  const detail = entry.details.find((item) => item.name === "ServiceName")
  const text = detail === undefined ? "" : detailValue(detail).trim()
  return text === "" ? null : text
}

export const defaultContextLines = 50
export const contextStep = 10
export const maxContextLines = 500

export const contextWindows = [
  { ms: 600_000, label: "10 minutes" },
  { ms: 3_600_000, label: "1 hour" },
  { ms: 86_400_000, label: "24 hours" },
] as const

export const defaultContextWindowMs = 3_600_000

const matchEveryDocument = "*"
const nanosPerMilli = 1_000_000n
const wholeMillisecondNanos = /^\d+000000$/

export type LogContext = components["schemas"]["LogContext"]

export function contextQueryFor(
  type: string,
  entry: LogEntry,
  streamLabels: Labels,
  indexed: readonly string[] | undefined
): string | null {
  if (type === "loki") return lokiStreamSelector(streamLabels, indexed)
  if (isSearchFamily(type)) return matchEveryDocument
  if (type !== "clickhouse") return null
  const service = serviceNameOf(entry)
  if (service === null) return clickhouseLogsTemplate
  return clickhouseLogsTemplate.replace(
    "\nORDER BY",
    ` AND ${matcherInsert("clickhouse", "ServiceName", service)}\nORDER BY`
  )
}

export function anchorNs(entry: LogEntry): string | null {
  const detail = entry.details.find((item) => item.name === "TimeNs")
  const text = detail === undefined ? "" : detailValue(detail).trim()
  if (text !== "") return text
  if (entry.time === null) return null
  return String(BigInt(Math.round(entry.time)) * nanosPerMilli)
}

export function anchorMatcher(
  anchor: LogEntry
): (candidate: LogEntry) => boolean {
  const ns = anchorNs(anchor)
  return (candidate) =>
    candidate.message === anchor.message && anchorNs(candidate) === ns
}

export function anchorFrames(entry: LogEntry): WireFrame[] {
  const detail = entry.details.find((item) => "frame" in item)
  if (detail === undefined || !("frame" in detail)) return []
  const { frame, index } = detail
  return keepRows(
    [frame],
    (candidate, row) => candidate === frame && row === index
  )
}

function oldestFirst(frames: readonly WireFrame[]): WireFrame[] {
  return frames
    .map((frame) => ({
      ...frame,
      fields: frame.fields.map((field) => ({
        ...field,
        values: [...field.values].reverse(),
      })),
    }))
    .reverse()
}

export function contextFrames(
  context: LogContext,
  anchor: readonly WireFrame[]
): WireFrame[] {
  return [
    ...oldestFirst(context.before.frames),
    ...anchor,
    ...context.after.frames,
  ]
}

export function moreLines(count: number): number {
  return Math.min(count + contextStep, maxContextLines)
}

export function dateFieldNote(
  type: string,
  frames: readonly WireFrame[]
): boolean {
  if (!isSearchFamily(type)) return false
  return frames.some((frame) =>
    frame.fields.some(
      (field) =>
        field.name === "TimeNs" &&
        field.values.some(
          (value) =>
            typeof value === "string" && wholeMillisecondNanos.test(value)
        )
    )
  )
}

function contextRequest(
  uid: string,
  body: ContextParams,
  signal?: AbortSignal
): Promise<LogContext> {
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/log-context", {
          params: { path: { uid } },
          body,
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function contextQuery(uid: string, body: ContextParams) {
  return queryOptions({
    queryKey: exploreKeys.context(uid, body),
    queryFn: ({ signal }) => contextRequest(uid, body, signal),
    staleTime: Infinity,
    retry: false,
    placeholderData: keepPreviousData,
  })
}
