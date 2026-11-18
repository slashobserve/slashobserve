import { createFileRoute, redirect } from "@tanstack/react-router"

import { datasourcesQuery } from "@/datasources/queries"
import { recentSources } from "@/datasources/datasource-memory"
import { ExploreList } from "@/datasources/explore-list"
import {
  carriesSearch,
  defaultSource,
  sourcesOf,
  withoutSource,
} from "@/datasources/sources"
import { validateMetricsEntrySearch } from "@/metrics/search"

export const Route = createFileRoute("/_authed/_shell/metrics/")({
  validateSearch: validateMetricsEntrySearch,
  staticData: { crumbs: [{ label: "Metrics" }] },
  beforeLoad: async ({ search, context }) => {
    const { uid, search: rest } = withoutSource(search)
    if (uid !== undefined) {
      throw redirect({
        to: "/metrics/$datasourceUid",
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
            "metrics",
            sourcesOf("metrics", listed.datasources),
            recentSources("metrics")
          )
    if (target !== undefined) {
      throw redirect({
        to: "/metrics/$datasourceUid",
        params: { datasourceUid: target.uid },
        search: rest,
        replace: true,
      })
    }
  },
  component: MetricsListRoute,
})

function MetricsListRoute() {
  return <ExploreList kind="metrics" />
}
