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
import { validateTraceEntrySearch } from "@/traces/search"

export const Route = createFileRoute("/_authed/_shell/traces/")({
  validateSearch: validateTraceEntrySearch,
  staticData: { crumbs: [{ label: "Traces" }] },
  beforeLoad: async ({ search, context }) => {
    const { uid, search: rest } = withoutSource(search)
    if (uid !== undefined) {
      throw redirect({
        to: "/traces/$datasourceUid",
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
            "traces",
            sourcesOf("traces", listed.datasources),
            recentSources("traces")
          )
    if (target !== undefined) {
      throw redirect({
        to: "/traces/$datasourceUid",
        params: { datasourceUid: target.uid },
        search: rest,
        replace: true,
      })
    }
  },
  component: TracesListRoute,
})

function TracesListRoute() {
  return (
    <div className="mx-auto w-full max-w-6xl">
      <ExploreList kind="traces" />
    </div>
  )
}
