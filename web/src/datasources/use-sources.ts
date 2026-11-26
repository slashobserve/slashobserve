import { useMemo } from "react"
import { useParams, useRouterState, useSearch } from "@tanstack/react-router"

import type { LogsSearch } from "@/logs/search"
import type { MetricsSearch } from "@/metrics/search"
import type { TraceSearch } from "@/traces/search"

import { useSourceMemory } from "./datasource-memory"
import { useDatasources, type DatasourceStatus } from "./queries"
import {
  browsableSources,
  carriedRange,
  exploreKindOfPath,
  isSourcePage,
  listedSources,
  logsSwitchSearch,
  metricsSwitchSearch,
  orderSources,
  sourcesOf,
  tracesSwitchSearch,
  type ExploreKind,
} from "./sources"

export function useExploreSources(kind: ExploreKind) {
  const { data, isPending, isError, error, refetch } = useDatasources()
  const recentAtLoad = useSourceMemory((state) => state.atLoad[kind])
  const sources = useMemo(
    () => orderSources(sourcesOf(kind, data), recentAtLoad),
    [kind, data, recentAtLoad]
  )
  return { sources, isPending, isError, error, refetch }
}

export function useBrowsableSources(kind: ExploreKind) {
  const { sources, ...rest } = useExploreSources(kind)
  const browsable = useMemo(
    () => browsableSources(kind, sources),
    [kind, sources]
  )
  return { sources: browsable, ...rest }
}

export function useRouteSource(kind: ExploreKind): {
  current: DatasourceStatus | undefined
  sources: DatasourceStatus[]
} {
  const { sources } = useExploreSources(kind)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { datasourceUid } = useParams({ strict: false })
  const current = isSourcePage(kind, pathname)
    ? sources.find((source) => source.uid === datasourceUid)
    : undefined
  return { current, sources }
}

export function useSourceSwitchSearch(
  kind: ExploreKind,
  targetType: string
): LogsSearch | TraceSearch | MetricsSearch {
  const { current } = useRouteSource(kind)
  const search = useSearch({ strict: false })
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  if (current === undefined) {
    return exploreKindOfPath(pathname) === undefined ? {} : carriedRange(search)
  }
  if (kind === "logs") {
    return logsSwitchSearch(search as LogsSearch, current.type, targetType)
  }
  if (kind === "traces") return tracesSwitchSearch(search as TraceSearch)
  return metricsSwitchSearch(search as MetricsSearch)
}

export function useListedSources(kind: ExploreKind) {
  const { sources, isPending } = useBrowsableSources(kind)
  return { ...listedSources(sources), pending: isPending }
}
