import type { ExploreParams } from "@/api/keys"
import type { WireFrame } from "@/dashboards/queries"
import { matcherInsert } from "@/panel-editor/insert-text"
import type { Labels, LogEntry } from "@/panels/logs/logs-model"

import { keepRows, rowKey, rowTime } from "./frames"
import { detailValue } from "./trace-link"

export const clickhouseLogsTemplate = `SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes
FROM otel_logs
WHERE $__timeFilter(Timestamp)
ORDER BY Timestamp DESC
LIMIT 1000`

const clickhouseLogColumns =
  "SELECT Timestamp, SeverityText, Body, ServiceName, TraceId, SpanId, LogAttributes\nFROM otel_logs"

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

const contextLines = 50
const beforeWindowMs = 3_600_000
const afterWindowsMs = [10_000, 60_000, 600_000, 3_600_000]
export const rowsPerAfterStep = 1000

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

export function lokiBeforeQuery(
  selector: string,
  anchorMs: number
): ExploreParams {
  return {
    query: selector,
    kind: "instant",
    from: String(anchorMs - beforeWindowMs),
    to: String(anchorMs + 1),
    limit: contextLines + 1,
  }
}

export function lokiAfterQuery(
  selector: string,
  anchorMs: number,
  step: number
): ExploreParams {
  return {
    query: selector,
    kind: "instant",
    from: String(anchorMs),
    to: String(anchorMs + afterWindowsMs[step]),
    limit: rowsPerAfterStep,
  }
}

export function nextAfterStep(
  step: number,
  linesAfter: number,
  saturated: boolean
): number | null {
  if (linesAfter >= contextLines || saturated) return null
  return step + 1 < afterWindowsMs.length ? step + 1 : null
}

function clickhouseContextSql(serviceName: string, order: "ASC" | "DESC") {
  return `${clickhouseLogColumns}
WHERE $__timeFilter(Timestamp) AND ${matcherInsert("clickhouse", "ServiceName", serviceName)}
ORDER BY Timestamp ${order}
LIMIT ${contextLines + 1}`
}

export function clickhouseContextQueries(
  serviceName: string,
  anchorMs: number
): { before: ExploreParams; after: ExploreParams } {
  return {
    before: {
      query: clickhouseContextSql(serviceName, "DESC"),
      kind: "instant",
      from: String(anchorMs - beforeWindowMs),
      to: String(anchorMs + 1),
    },
    after: {
      query: clickhouseContextSql(serviceName, "ASC"),
      kind: "instant",
      from: String(anchorMs),
      to: String(anchorMs + beforeWindowMs),
    },
  }
}

export type ContextAnchor = { time: number; key: string | null }

type Row = { id: string; time: number; key: string }

export function contextFrames(
  frames: readonly WireFrame[],
  anchor: ContextAnchor,
  count = contextLines
): WireFrame[] {
  const seen = new Set<string>()
  const rows: Row[] = []
  frames.forEach((frame, index) => {
    const length = Math.max(0, ...frame.fields.map((f) => f.values.length))
    for (let row = 0; row < length; row++) {
      const time = rowTime(frame, row)
      const key = rowKey(frame, row)
      if (time === null || seen.has(key)) continue
      seen.add(key)
      rows.push({ id: `${index}:${row}`, time, key })
    }
  })
  rows.sort((a, b) => a.time - b.time)
  const matched = rows.findIndex(
    (row) => row.time === anchor.time && row.key === anchor.key
  )
  const kept = new Set<string>()
  if (matched >= 0) {
    kept.add(rows[matched].id)
    rows
      .slice(Math.max(0, matched - count), matched)
      .forEach((row) => kept.add(row.id))
    rows
      .slice(matched + 1, matched + 1 + count)
      .forEach((row) => kept.add(row.id))
  } else {
    const before = rows.filter((row) => row.time < anchor.time)
    const at = rows.filter((row) => row.time === anchor.time)
    const after = rows.filter((row) => row.time > anchor.time)
    at.forEach((row) => kept.add(row.id))
    before
      .slice(Math.max(0, before.length - count))
      .forEach((row) => kept.add(row.id))
    after.slice(0, count).forEach((row) => kept.add(row.id))
  }
  const indexOf = new Map(frames.map((frame, index) => [frame, index]))
  return keepRows(frames, (frame, row) =>
    kept.has(`${indexOf.get(frame)}:${row}`)
  )
}

export function serviceNameOf(entry: LogEntry): string | null {
  const detail = entry.details.find((item) => item.name === "ServiceName")
  const text = detail === undefined ? "" : detailValue(detail).trim()
  return text === "" ? null : text
}

export function anchorRowKey(entry: LogEntry): string | null {
  const detail = entry.details.find((item) => "frame" in item)
  return detail !== undefined && "frame" in detail
    ? rowKey(detail.frame, detail.index)
    : null
}
