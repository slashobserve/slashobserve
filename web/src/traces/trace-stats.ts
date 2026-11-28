import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { isRetryable, unwrap } from "@/api/errors"
import { exploreKeys, type TraceStatsParams } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { panelRequestLimiter } from "@/dashboards/limiter"
import { zonedTimeValue } from "@/lib/time"

import type { TraceSearch } from "./search"
import { countText, type TraceRow } from "./trace-rows"

export type TraceStats = components["schemas"]["TraceStats"]

function traceStatsRequest(
  uid: string,
  params: TraceStatsParams,
  signal?: AbortSignal
): Promise<TraceStats> {
  return panelRequestLimiter.run(
    () =>
      api
        .POST("/datasources/{uid}/trace-stats", {
          params: { path: { uid } },
          body: params,
          ...(signal === undefined ? {} : { signal }),
        })
        .then(unwrap),
    signal
  )
}

export function traceStatsQuery(uid: string, params: TraceStatsParams) {
  return queryOptions({
    queryKey: exploreKeys.traceStats(uid, params),
    queryFn: ({ signal }) => traceStatsRequest(uid, params, signal),
    staleTime: 30_000,
    retry: (count, error) => count < 1 && isRetryable(error),
    refetchOnWindowFocus: false,
  })
}

export function histogramPercentile(
  histogram: TraceStats["histogram"],
  p: number
): number | null {
  const total = histogram.reduce((sum, bucket) => sum + bucket.count, 0)
  if (total === 0) return null
  const wanted = (p / 100) * total
  let seen = 0
  for (const bucket of histogram) {
    seen += bucket.count
    if (seen >= wanted) return bucket.le_ms
  }
  return histogram[histogram.length - 1]?.le_ms ?? null
}

export function p95Label(search: TraceSearch): string {
  return search.service === undefined
    ? "p95 of all traces"
    : `p95 of all ${search.service} traces`
}

export function partialReason({
  order,
  rowCount,
  pageSize,
  hasNext,
  cut,
}: {
  order: string
  rowCount: number
  pageSize: number
  hasNext: boolean
  cut: boolean
}): string {
  if (cut && (rowCount < pageSize || !hasNext)) {
    return hasNext
      ? "Tempo stopped searching at its time limit, so these are the newest matches it found; the next page searches further back."
      : "Tempo stopped searching at its time limit, so some matching traces may be missing."
  }
  const shown = `This page shows ${countText(rowCount, "trace")}, ${order}`
  return hasNext
    ? `${shown}; more matches are on the next pages.`
    : `${shown}; the rest are on the earlier pages.`
}

export function noTracesText(partial: boolean, range: string): string {
  return partial
    ? `Tempo stopped searching at its time limit before it found a trace in ${range}. Choose a shorter range or add a filter.`
    : `No traces match in ${range}.`
}

export function partialNote(
  rows: readonly TraceRow[],
  zone: string | undefined
): { text: string; covers: string; from: string; to: string } | null {
  const starts = rows.flatMap((row) => (row.start === null ? [] : [row.start]))
  if (starts.length === 0) return null
  const earliest = Math.min(...starts)
  const latest = Math.max(...starts)
  const at = (ms: number) => zonedTimeValue(String(ms), zone)
  const covers = `${at(earliest)} to ${at(latest)}`
  return {
    text: `The ${rows.length.toLocaleString("en")} found cover ${covers}.`,
    covers,
    from: new Date(earliest).toISOString(),
    to: new Date(latest).toISOString(),
  }
}
