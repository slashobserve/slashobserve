import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import {
  firstString,
  validAsk,
  validDatasource,
  validObservation,
  validQuery,
  validRange,
  validSaved,
} from "@/logs/search"

export type MetricsMode = "graph" | "table"

export type MetricsSearch = {
  q?: string
  ask?: string
  from?: string
  to?: string
  mode?: "table"
  observation?: string
  saved?: string
}

export const metricsDatasourceTypes = ["prometheus"] as const

export function validateMetricsSearch(
  raw: Record<string, unknown>
): MetricsSearch {
  return {
    q: validQuery(raw.q),
    ask: validAsk(raw.ask),
    ...validRange(raw),
    mode: firstString(raw.mode) === "table" ? "table" : undefined,
    observation: validObservation(raw.observation),
    saved: validSaved(raw.saved),
  }
}

export type MetricsEntrySearch = MetricsSearch & { ds?: string }

export function validateMetricsEntrySearch(
  raw: Record<string, unknown>
): MetricsEntrySearch {
  return { ...validateMetricsSearch(raw), ds: validDatasource(raw.ds) }
}

export function metricsTimeRange(search: MetricsSearch): TimeRangeValue {
  return { from: search.from ?? "now-1h", to: search.to ?? "now" }
}

export function modeOf(search: MetricsSearch): MetricsMode {
  return search.mode === "table" ? "table" : "graph"
}

export function searchAfter(
  state: MetricsSearch,
  next: Partial<MetricsSearch>
): MetricsSearch {
  return { ...state, ...next }
}
