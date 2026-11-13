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
import { validateLogsEntrySearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/")({
  validateSearch: validateLogsEntrySearch,
  staticData: { crumbs: [{ label: "Logs" }] },
  beforeLoad: async ({ search, context }) => {
    const { uid, search: rest } = withoutSource(search)
    if (uid !== undefined) {
      throw redirect({
        to: "/logs/$datasourceUid",
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
            "logs",
            sourcesOf("logs", listed.datasources),
            recentSources("logs")
          )
    if (target !== undefined) {
      throw redirect({
        to: "/logs/$datasourceUid",
        params: { datasourceUid: target.uid },
        search: rest,
        replace: true,
      })
    }
  },
  component: LogsListRoute,
})

function LogsListRoute() {
  return <ExploreList kind="logs" />
}
