import { useMemo } from "react"
import { useParams, useRouterState, useSearch } from "@tanstack/react-router"

import type { LogsSearch } from "@/logs/search"
import type { TraceSearch } from "@/traces/search"

import { useSourceMemory } from "./datasource-memory"
import { useDatasources, type DatasourceStatus } from "./queries"
import {
  listedSources,
  logsSwitchSearch,
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

export function useRouteSource(kind: ExploreKind): {
  current: DatasourceStatus | undefined
  sources: DatasourceStatus[]
} {
  const { sources } = useExploreSources(kind)
  const pathname = useRouterState({
    select: (state) => state.location.pathname,
  })
  const { datasourceUid } = useParams({ strict: false })
  const onSourcePage = pathname.startsWith(`/${kind}/`)
  const current = onSourcePage
    ? sources.find((source) => source.uid === datasourceUid)
    : undefined
  return { current, sources }
}

export function useSourceSwitchSearch(
  kind: ExploreKind,
  targetType: string
): LogsSearch | TraceSearch {
  const { current } = useRouteSource(kind)
  const search = useSearch({ strict: false })
  if (current === undefined) return {}
  return kind === "logs"
    ? logsSwitchSearch(search as LogsSearch, current.type, targetType)
    : tracesSwitchSearch(search as TraceSearch)
}

export function useListedSources(kind: ExploreKind) {
  return listedSources(useExploreSources(kind).sources)
}
