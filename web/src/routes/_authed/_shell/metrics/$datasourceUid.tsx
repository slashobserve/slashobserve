import { createFileRoute, redirect } from "@tanstack/react-router"

import { validateMetricsSearch } from "@/metrics/search"

export const Route = createFileRoute("/_authed/_shell/metrics/$datasourceUid")({
  validateSearch: validateMetricsSearch,
  beforeLoad: ({ params, search }) => {
    throw redirect({
      to: "/explore/metrics/$datasourceUid",
      params,
      search: validateMetricsSearch(search),
      replace: true,
    })
  },
})
