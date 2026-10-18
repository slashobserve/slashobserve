import { queryOptions } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"

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
