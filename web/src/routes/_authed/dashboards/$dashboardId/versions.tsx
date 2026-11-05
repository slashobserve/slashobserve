import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateHistorySearch } from "@/history/search"

export const Route = createFileRoute(
  "/_authed/dashboards/$dashboardId/versions"
)({
  beforeLoad: ({ location, params }) => {
    throw redirect({
      to: "/d/$dashboardId/versions",
      params,
      search: validateHistorySearch(location.search),
      replace: true,
      statusCode: 301,
    })
  },
})
