import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"
import type { components } from "@/api/schema.gen"
import { latestDashboardQuery } from "@/dashboards/queries"

export type VersionMeta = components["schemas"]["VersionMeta"]

const pageSize = 50

export function useVersionHistory(id: string) {
  return useInfiniteQuery({
    queryKey: dashboardKeys.history(id),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/dashboards/{id}/versions", {
          params: {
            path: { id },
            query: { limit: pageSize, cursor: pageParam },
          },
          signal,
        })
        .then(unwrap),
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 15_000,
  })
}

export function useKnownLatestVersion(id: string): number {
  const queryClient = useQueryClient()
  const latest = useQuery(latestDashboardQuery(queryClient, id))
  const history = useVersionHistory(id)
  return Math.max(
    latest.data?.dashboard.latest_version ?? 0,
    history.data?.pages[0]?.versions[0]?.version ?? 0
  )
}
