import { logsPath, tracesPath } from "@/lib/links"
import { eligibleDatasources } from "@/logs/datasource-picker"
import {
  isSearchFamily,
  logsDatasourceTypes,
  queryModeLabel,
  type LogsSearch,
} from "@/logs/search"
import type { SubNavRow } from "@/shell/nav"
import { traceDatasourceTypes, type TraceSearch } from "@/traces/search"

import type { DatasourceStatus } from "./queries"

export type ExploreKind = "logs" | "traces"

const listedSourceCap = 5

export function sourcesOf(
  kind: ExploreKind,
  list: readonly DatasourceStatus[] | undefined
): DatasourceStatus[] {
  return eligibleDatasources(
    list,
    kind === "logs" ? logsDatasourceTypes : traceDatasourceTypes
  )
}

export function sourceLanguage(kind: ExploreKind, type: string): string {
  if (kind === "logs") return queryModeLabel(type)
  if (type === "tempo") return "TraceQL"
  if (type === "clickhouse") return "Search"
  return "Query"
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
  if (wasUsed || kind === "logs") return ordered[0]
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
  const pathOf = kind === "logs" ? logsPath : tracesPath
  return sources.map(({ uid }) => ({ id: uid, paths: [pathOf(uid)] }))
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
    page: undefined,
  }
}

export function tracesSwitchSearch(current: TraceSearch): TraceSearch {
  return { ...current, ask: undefined, page: undefined }
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
