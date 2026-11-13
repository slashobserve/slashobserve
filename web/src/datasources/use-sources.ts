import { useMemo } from "react"
import { useParams, useRouterState, useSearch } from "@tanstack/react-router"

import { logsLink, tracesLink } from "@/lib/links"
import type { LogsSearch } from "@/logs/search"
import type { NavPath } from "@/shell/nav"
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
  const recent = useSourceMemory((state) => state[kind])
  const sources = useMemo(
    () => orderSources(sourcesOf(kind, data), recent),
    [kind, data, recent]
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

type NavTarget =
  { to: NavPath } | ReturnType<typeof logsLink> | ReturnType<typeof tracesLink>

function exploreKindOf(to: NavPath): ExploreKind | null {
  if (to === "/logs") return "logs"
  if (to === "/traces") return "traces"
  return null
}

function useNavSources(to: NavPath) {
  const kind = exploreKindOf(to)
  const { sources } = useExploreSources(kind ?? "logs")
  return { kind, sources: kind === null ? [] : sources }
}

export function useNavTarget(to: NavPath): NavTarget {
  const { kind, sources } = useNavSources(to)
  const [only] = sources
  if (kind === null || sources.length !== 1 || only === undefined) return { to }
  return kind === "logs" ? logsLink(only.uid) : tracesLink(only.uid)
}

export function useListedSources(kind: ExploreKind) {
  return listedSources(useExploreSources(kind).sources)
}
