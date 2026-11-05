import { createFileRoute, redirect } from "@tanstack/react-router"

import { searchFromState, validateDashboardSearch } from "@/dashboards/search"
import { dashboardLink } from "@/lib/links"

export const Route = createFileRoute("/_authed/dashboards/$dashboardId/")({
  beforeLoad: ({ location, params }) => {
    throw redirect({
      ...dashboardLink(params.dashboardId),
      search: searchFromState(validateDashboardSearch(location.search)),
      hash: location.hash,
      replace: true,
      statusCode: 301,
    })
  },
})
