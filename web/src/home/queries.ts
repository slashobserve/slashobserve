import { useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { homeKeys, type HomeSearch } from "@/api/keys"

const firstDashboardsLimit = 8
const searchLimit = 20

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
              q: search.q,
              tag: search.tag.length > 0 ? search.tag : undefined,
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
