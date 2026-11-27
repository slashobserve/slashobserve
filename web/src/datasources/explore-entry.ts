import type { QueryClient } from "@tanstack/react-query"
import { redirect } from "@tanstack/react-router"

import { recentSources } from "./datasource-memory"
import { datasourcesQuery } from "./queries"
import {
  browsableSources,
  carriesSearch,
  defaultSource,
  sourcesOf,
  withoutSource,
  type ExploreKind,
} from "./sources"

const entryRoutes = {
  logs: "/logs/$datasourceUid",
  traces: "/traces/$datasourceUid",
  metrics: "/explore/metrics/$datasourceUid",
} as const

export function exploreEntryBeforeLoad(kind: ExploreKind) {
  const to = entryRoutes[kind]
  return async ({
    search,
    context,
  }: {
    search: { ds?: string }
    context: { queryClient: QueryClient }
  }) => {
    const { uid, search: rest } = withoutSource(search)
    if (uid !== undefined) {
      throw redirect({
        to,
        params: { datasourceUid: uid },
        search: rest,
        replace: true,
      })
    }
    if (!carriesSearch(rest)) return
    const listed = await context.queryClient
      .ensureQueryData(datasourcesQuery())
      .catch(() => undefined)
    const target =
      listed === undefined
        ? undefined
        : defaultSource(
            kind,
            browsableSources(kind, sourcesOf(kind, listed.datasources)),
            recentSources(kind)
          )
    if (target !== undefined) {
      throw redirect({
        to,
        params: { datasourceUid: target.uid },
        search: rest,
        replace: true,
      })
    }
  }
}
