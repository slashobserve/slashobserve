import { createFileRoute } from "@tanstack/react-router"

import { MetricsPage } from "@/metrics/metrics-page"
import { validateMetricsSearch } from "@/metrics/search"

export const Route = createFileRoute("/_authed/_shell/metrics/$datasourceUid")({
  validateSearch: validateMetricsSearch,
  staticData: {
    crumbs: [{ label: "Metrics", to: "/metrics" }],
    pageTitle: true,
  },
  component: MetricsSourceRoute,
})

function MetricsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <MetricsPage uid={datasourceUid} />
}
