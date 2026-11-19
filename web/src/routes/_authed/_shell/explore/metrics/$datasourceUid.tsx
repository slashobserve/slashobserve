import { createFileRoute } from "@tanstack/react-router"

import { MetricsPage } from "@/metrics/metrics-page"
import { validateMetricsSearch } from "@/metrics/search"

export const Route = createFileRoute(
  "/_authed/_shell/explore/metrics/$datasourceUid"
)({
  validateSearch: validateMetricsSearch,
  staticData: {
    crumbs: [{ label: "Metrics", to: "/explore/metrics" }],
    pageTitle: true,
  },
  component: MetricsSourceRoute,
})

function MetricsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <MetricsPage uid={datasourceUid} />
}
