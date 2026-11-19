import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"

const heartbeatMs = 20_000

function dashboardViewersQuery(dashboardId: string) {
  return queryOptions({
    queryKey: dashboardKeys.viewers(dashboardId),
    queryFn: ({ signal }) =>
      api
        .POST("/dashboards/{id}/viewers", {
          params: { path: { id: dashboardId } },
          signal,
        })
        .then(unwrap),
    select: (envelope) => envelope.viewers,
    refetchInterval: heartbeatMs,
    refetchIntervalInBackground: false,
    retry: false,
  })
}

export function useDashboardViewers(dashboardId: string) {
  return useQuery(dashboardViewersQuery(dashboardId)).data ?? []
}
