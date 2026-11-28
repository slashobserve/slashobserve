import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { homeKeys, type HomeSearch } from "@/api/keys"

const firstDashboardsLimit = 8
const searchLimit = 20
const workingPollMs = 5000
const latestChangeStaleMs = 60_000

export function workingRunsQuery() {
  return queryOptions({
    queryKey: homeKeys.working(),
    queryFn: ({ signal }) =>
      api.GET("/agents/working", { signal }).then(unwrap),
    staleTime: 10_000,
    refetchInterval: (query) =>
      (query.state.data?.items.length ?? 0) > 0 ? workingPollMs : false,
  })
}

export function latestChangeQuery(observationId: string) {
  return queryOptions({
    queryKey: homeKeys.latestChange(observationId),
    queryFn: ({ signal }) =>
      api
        .GET("/activity", {
          params: { query: { observation_id: observationId, limit: 1 } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.activity[0] ?? null),
    staleTime: latestChangeStaleMs,
  })
}

export function useFirstDashboards(enabled: boolean) {
  return useQuery({
    queryKey: homeKeys.firstDashboards(),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: { query: { order: "folder", limit: firstDashboardsLimit } },
          signal,
        })
        .then(unwrap)
        .then((body) => body.dashboards),
    enabled,
    staleTime: 15_000,
  })
}

export function useDashboardSearch(search: HomeSearch, enabled: boolean) {
  return useQuery({
    queryKey: homeKeys.search(search),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards", {
          params: {
            query: {
              ...(search.q === undefined ? {} : { q: search.q }),
              ...(search.tag.length > 0 ? { tag: search.tag } : {}),
              limit: searchLimit,
            },
          },
          signal,
        })
        .then(unwrap)
        .then((body) => body.dashboards),
    enabled,
    staleTime: 15_000,
    placeholderData: (previous) => previous,
  })
}
