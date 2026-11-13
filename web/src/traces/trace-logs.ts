import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { exploreKeys, type TraceLogsParams } from "@/api/keys"
import { panelRequestLimiter } from "@/dashboards/limiter"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { logsUrl } from "@/lib/links"
import { requestZone } from "@/lib/time"
import { clickhouseLogsTemplate } from "@/logs/context-query"
import { rowTime } from "@/logs/frames"
import type { LogsSearch } from "@/logs/search"
import { matcherInsert } from "@/panel-editor/insert-text"

export type TraceLogsWindow = { from: string; to: string }

export type TraceLogLine = { time: number | null; body: string }

const storageKey = "slashobserve-trace-logs-datasource"
const logsTypes = new Set(["loki", "clickhouse"])
const paddingMs = 60_000
const lineLimit = 500
const bodyNames = new Set(["body", "message", "msg", "line", "log"])
const lokiSelector = '{service_name=~".+"}'
const clickhouseTimeFilter = "WHERE $__timeFilter(Timestamp)"

function traceLogsRequest(
  uid: string,
  { traceId, spanId, service, from, to, limit }: TraceLogsParams,
  signal?: AbortSignal
): Promise<PanelDataResponse> {
  const timezone = requestZone("browser")
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/trace-logs", {
          params: { path: { uid } },
          body: {
            trace_id: traceId,
            ...(spanId !== undefined && { span_id: spanId }),
            ...(service !== undefined && service !== "" && { service }),
            from,
            to,
            ...(timezone !== undefined && { timezone }),
            ...(limit !== undefined && { limit }),
          },
          signal,
        })
        .then(unwrap),
    signal
  )
}

export function traceLogsQuery(uid: string, params: TraceLogsParams) {
  return queryOptions({
    queryKey: exploreKeys.traceLogs(uid, params),
    queryFn: ({ signal }) => traceLogsRequest(uid, params, signal),
    staleTime: 60_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function logsDatasourceCandidates(
  all: readonly DatasourceStatus[]
): DatasourceStatus[] {
  return all.filter((datasource) => logsTypes.has(datasource.type))
}

function storedLogsDatasource(): string | null {
  try {
    return localStorage.getItem(storageKey)
  } catch {
    return null
  }
}

export function rememberLogsDatasource(uid: string): void {
  try {
    localStorage.setItem(storageKey, uid)
  } catch {
    return
  }
}

function sameFamilyType(traceType: string | undefined): string {
  return traceType === "clickhouse" ? "clickhouse" : "loki"
}

export function logsDatasourceFor(
  traceUid: string,
  all: readonly DatasourceStatus[]
): DatasourceStatus | null {
  const candidates = logsDatasourceCandidates(all)
  if (candidates.length === 0) return null
  if (candidates.length === 1) return candidates[0]
  const stored = storedLogsDatasource()
  const remembered = candidates.find((candidate) => candidate.uid === stored)
  if (remembered) return remembered
  const traceType = all.find((datasource) => datasource.uid === traceUid)?.type
  const wanted = sameFamilyType(traceType)
  return (
    candidates.find((candidate) => candidate.type === wanted) ?? candidates[0]
  )
}

export function traceLogsWindow(
  start: number,
  duration: number
): TraceLogsWindow {
  return {
    from: String(Math.floor(start - paddingMs)),
    to: String(Math.ceil(start + duration + paddingMs)),
  }
}

function lokiTraceQuery(traceId: string, spanId: string | undefined): string {
  const filters = [matcherInsert("loki", "trace_id", traceId)]
  if (spanId !== undefined)
    filters.push(matcherInsert("loki", "span_id", spanId))
  return [lokiSelector, ...filters].join(" | ")
}

function clickhouseTraceQuery(
  traceId: string,
  spanId: string | undefined
): string {
  const conditions = [matcherInsert("clickhouse", "TraceId", traceId)]
  if (spanId !== undefined) {
    conditions.push(matcherInsert("clickhouse", "SpanId", spanId))
  }
  return clickhouseLogsTemplate.replace(
    clickhouseTimeFilter,
    [clickhouseTimeFilter, ...conditions].join(" AND ")
  )
}

export function traceLogsSearch(
  type: string,
  traceId: string,
  window: TraceLogsWindow,
  spanId?: string
): LogsSearch {
  return {
    q:
      type === "clickhouse"
        ? clickhouseTraceQuery(traceId, spanId)
        : lokiTraceQuery(traceId, spanId),
    from: window.from,
    to: window.to,
  }
}

export function traceLogsAddress(
  uid: string,
  type: string,
  traceId: string,
  window: TraceLogsWindow,
  spanId?: string
): string {
  return logsUrl(uid, traceLogsSearch(type, traceId, window, spanId))
}

function bodyField(frame: WireFrame) {
  return (
    frame.fields.find((field) => bodyNames.has(field.name.toLowerCase())) ??
    frame.fields.find((field) => field.type === "string")
  )
}

export function traceLogLines(frames: readonly WireFrame[]): TraceLogLine[] {
  const lines: TraceLogLine[] = []
  for (const frame of frames) {
    const body = bodyField(frame)
    const rows = Math.max(
      0,
      ...frame.fields.map((field) => field.values.length)
    )
    for (let row = 0; row < rows; row++) {
      lines.push({
        time: rowTime(frame, row),
        body: String(body?.values[row] ?? ""),
      })
    }
  }
  return lines.sort((a, b) => (a.time ?? 0) - (b.time ?? 0))
}

export function lineCountText(count: number, truncated: boolean): string {
  if (truncated) return `${lineLimit}+ lines`
  return count === 1 ? "1 line" : `${count.toLocaleString("en")} lines`
}

export const truncationNote = `Showing the newest ${lineLimit} lines.`

export function previewTime(ms: number | null): string {
  if (ms === null) return "-"
  const date = new Date(ms)
  const seconds = String(date.getUTCSeconds()).padStart(2, "0")
  const millis = String(date.getUTCMilliseconds()).padStart(3, "0")
  return `${seconds}.${millis}`
}

export function matchNote(
  type: string,
  span: { service: string } | undefined
): string {
  if (type === "clickhouse") {
    return span
      ? "Matched on TraceId and SpanId in otel_logs."
      : "Matched on TraceId in otel_logs."
  }
  if (!span) return "Matched on trace_id in every stream."
  return span.service === ""
    ? "Matched on trace_id and span_id in every stream."
    : `Matched on trace_id and span_id in the ${span.service} stream.`
}
