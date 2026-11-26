import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateTraceSearch } from "@/traces/search"

export const Route = createFileRoute(
  "/_authed/_shell/explore/traces/$datasourceUid"
)({
  validateSearch: validateTraceSearch,
  beforeLoad: ({ params, search }) => {
    throw redirect({
      to: "/traces/$datasourceUid",
      params,
      search,
      replace: true,
    })
  },
})
