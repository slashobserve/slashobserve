import { keepPreviousData, queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { ApiError, unwrap } from "@/api/errors"
import { dashboardKeys, searchKeys } from "@/api/keys"

export function dashboardTitlesQuery() {
  return queryOptions({
    queryKey: dashboardKeys.titles(),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", { params: { query: { limit: 200 } }, signal })
        .then(unwrap),
    select: (data) => data.dashboards.map(({ id, title }) => ({ id, title })),
    staleTime: 15_000,
  })
}

const searchLimit = 20

export function workspaceSearchQuery(q: string) {
  return queryOptions({
    queryKey: searchKeys.results(q),
    queryFn: ({ signal }) =>
      api
        .GET("/search", {
          params: { query: { q, limit: searchLimit } },
          signal,
        })
        .then(unwrap)
        .catch((error: unknown) => {
          if (error instanceof ApiError && error.status === 400) {
            return { results: [], next_cursor: null }
          }
          throw error
        }),
    select: (data) => data.results,
    staleTime: 15_000,
    placeholderData: keepPreviousData,
  })
}
