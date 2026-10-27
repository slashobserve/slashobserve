import { infiniteQueryOptions, queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { importKeys } from "@/api/keys"

export function myImportsQuery() {
  return infiniteQueryOptions({
    queryKey: importKeys.list("mine"),
    queryFn: ({ pageParam, signal }) =>
      api
        .GET("/imports", {
          params: {
            query: { made_by: "me", limit: 20, cursor: pageParam },
          },
          signal,
        })
        .then(unwrap),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 10_000,
  })
}

export function importForDashboardQuery(dashboardId: string) {
  return queryOptions({
    queryKey: importKeys.forDashboard(dashboardId),
    queryFn: async ({ signal }) => {
      const list = await api
        .GET("/imports", {
          params: { query: { dashboard_id: dashboardId, limit: 1 } },
          signal,
        })
        .then(unwrap)
      if (list.imports.length === 0) return null
      return api
        .GET("/dashboards/{id}/import", {
          params: { path: { id: dashboardId } },
          signal,
        })
        .then(unwrap)
    },
    staleTime: Infinity,
    retry: false,
  })
}
