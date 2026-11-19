import { queryOptions, useQuery } from "@tanstack/react-query"

import { api } from "@/api/client"
import { unwrap } from "@/api/errors"
import { dashboardKeys } from "@/api/keys"

function dashboardViewsQuery(id: string) {
  return queryOptions({
    queryKey: dashboardKeys.views(id),
    queryFn: ({ signal }) =>
      api
        .GET("/dashboards/{id}/views", { params: { path: { id } }, signal })
        .then(unwrap),
    staleTime: 60_000,
  })
}

export function viewsText(count: number, windowDays: number): string {
  return `${count} ${count === 1 ? "view" : "views"} in ${windowDays} days`
}

export function useViewsText(id: string): string | null {
  const views = useQuery(dashboardViewsQuery(id)).data
  return views ? viewsText(views.view_count, views.window_days) : null
}
