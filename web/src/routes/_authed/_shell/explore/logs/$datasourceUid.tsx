import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute(
  "/_authed/_shell/explore/logs/$datasourceUid"
)({
  validateSearch: validateLogsSearch,
  beforeLoad: ({ params, search }) => {
    throw redirect({
      to: "/logs/$datasourceUid",
      params,
      search,
      replace: true,
    })
  },
})
