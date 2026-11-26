import { auditDatasourceUid } from "@/audit/audit-source"
import {
  logsLink,
  logsPath,
  metricsLink,
  metricsPath,
  tracesLink,
  tracesPath,
} from "@/lib/links"
import { eligibleDatasources } from "@/logs/datasource-picker"
import {
  isSearchFamily,
  logsDatasourceTypes,
  queryModeLabel,
  type LogsSearch,
} from "@/logs/search"
import { metricsDatasourceTypes, type MetricsSearch } from "@/metrics/search"
import type { SubNavRow } from "@/shell/nav"
import { traceDatasourceTypes, type TraceSearch } from "@/traces/search"

import type { DatasourceStatus } from "./queries"

export type ExploreKind = "logs" | "traces" | "metrics"

export const exploreKinds = ["logs", "traces", "metrics"] as const

export const exploreText = {
  logs: { page: "Logs", word: "log", sources: "Log sources", path: "/logs" },
  traces: {
    page: "Traces",
    word: "trace",
    sources: "Trace sources",
    path: "/traces",
  },
  metrics: {
    page: "Metrics",
    word: "metric",
    sources: "Metric sources",
    path: "/explore/metrics",
  },
} as const

export function isSourcePage(kind: ExploreKind, pathname: string): boolean {
  return pathname.startsWith(`${exploreText[kind].path}/`)
}

const listedSourceCap = 5

const datasourceTypesOf: Record<ExploreKind, readonly string[]> = {
  logs: logsDatasourceTypes,
  traces: traceDatasourceTypes,
  metrics: metricsDatasourceTypes,
}

export function sourcesOf(
  kind: ExploreKind,
  list: readonly DatasourceStatus[] | undefined
): DatasourceStatus[] {
  return eligibleDatasources(list, datasourceTypesOf[kind])
}

export function browsableSources(
  kind: ExploreKind,
  sources: readonly DatasourceStatus[]
): DatasourceStatus[] {
  return kind === "logs"
    ? sources.filter((source) => source.uid !== auditDatasourceUid)
    : [...sources]
}

export function sourceLanguage(kind: ExploreKind, type: string): string {
  if (kind === "metrics") return "PromQL"
  if (kind === "logs") return queryModeLabel(type)
  if (type === "tempo") return "TraceQL"
  if (type === "clickhouse" || type === "jaeger") return "Search"
  return "Query"
}

export const kqlReferenceUrl =
  "https://slashobserve.com/docs/use/logs-and-traces#kql-on-elasticsearch-and-opensearch"

const referenceUrls: Record<string, string> = {
  KQL: kqlReferenceUrl,
  LogQL: "https://grafana.com/docs/loki/latest/query/",
  PromQL: "https://prometheus.io/docs/prometheus/latest/querying/basics/",
  SQL: "https://clickhouse.com/docs/sql-reference",
  TraceQL: "https://grafana.com/docs/tempo/latest/traceql/",
}

export function languageReference(
  kind: ExploreKind,
  type: string
): { label: string; url: string } | null {
  const language = sourceLanguage(kind, type)
  const url = referenceUrls[language]
  return url === undefined ? null : { label: `${language} reference`, url }
}

function byName(first: DatasourceStatus, second: DatasourceStatus): number {
  return (
    first.name.localeCompare(second.name, undefined, {
      sensitivity: "base",
    }) || first.uid.localeCompare(second.uid)
  )
}

export function orderSources(
  sources: readonly DatasourceStatus[],
  recent: readonly string[]
): DatasourceStatus[] {
  const used = recent.flatMap((uid) => {
    const source = sources.find((candidate) => candidate.uid === uid)
    return source === undefined ? [] : [source]
  })
  const rest = sources.filter((source) => !used.includes(source)).sort(byName)
  return [...used, ...rest]
}

export function defaultSource(
  kind: ExploreKind,
  sources: readonly DatasourceStatus[],
  recent: readonly string[]
): DatasourceStatus | undefined {
  const ordered = orderSources(sources, recent)
  const wasUsed = recent.some((uid) =>
    sources.some((source) => source.uid === uid)
  )
  if (wasUsed || kind !== "traces") return ordered[0]
  return ordered.find((source) => source.type === "tempo") ?? ordered[0]
}

export function listedSources(ordered: readonly DatasourceStatus[]): {
  shown: DatasourceStatus[]
  more: boolean
} {
  if (ordered.length < 2) return { shown: [], more: false }
  return {
    shown: ordered.slice(0, listedSourceCap),
    more: ordered.length > listedSourceCap,
  }
}

export function sourceRows(
  kind: ExploreKind,
  sources: readonly DatasourceStatus[]
): SubNavRow[] {
  const pathOf = { logs: logsPath, traces: tracesPath, metrics: metricsPath }[
    kind
  ]
  return sources.map(({ uid }) => ({ id: uid, paths: [pathOf(uid)] }))
}

export function exploreKindOfPath(pathname: string): ExploreKind | undefined {
  return exploreKinds.find((kind) => isSourcePage(kind, pathname))
}

export function carriedRange(search: {
  from?: string
  to?: string
}): { from: string; to: string } | Record<string, never> {
  return search.from !== undefined && search.to !== undefined
    ? { from: search.from, to: search.to }
    : {}
}

export function exploreLink(
  kind: ExploreKind,
  uid: string,
  search: LogsSearch | TraceSearch | MetricsSearch
) {
  if (kind === "logs") return logsLink(uid, search as LogsSearch)
  if (kind === "traces") return tracesLink(uid, search as TraceSearch)
  return metricsLink(uid, search as MetricsSearch)
}

export function logsSwitchSearch(
  current: LogsSearch,
  fromType: string,
  toType: string
): LogsSearch {
  const keepsQuery = isSearchFamily(fromType) && isSearchFamily(toType)
  return {
    ...current,
    q: keepsQuery ? current.q : undefined,
    ask: undefined,
    line: undefined,
    cols: undefined,
    saved: undefined,
    page: undefined,
  }
}

export function tracesSwitchSearch(current: TraceSearch): TraceSearch {
  return { ...current, ask: undefined, page: undefined }
}

export function metricsSwitchSearch(current: MetricsSearch): MetricsSearch {
  return { ...current, ask: undefined }
}

export function withoutSource<T extends { ds?: string }>(
  search: T
): { uid: string | undefined; search: Omit<T, "ds"> } {
  const { ds, ...rest } = search
  return { uid: ds, search: rest }
}

export function carriesSearch(search: Record<string, unknown>): boolean {
  return Object.values(search).some((value) => value !== undefined)
}
