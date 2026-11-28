import { createFileRoute } from "@tanstack/react-router"

import { exploreEntryBeforeLoad } from "@/datasources/explore-entry"
import { ExploreList } from "@/datasources/explore-list"
import { validateMetricsEntrySearch } from "@/metrics/search"

export const Route = createFileRoute("/_authed/_shell/explore/metrics/")({
  validateSearch: validateMetricsEntrySearch,
  staticData: { crumbs: [{ label: "Metrics" }] },
  beforeLoad: exploreEntryBeforeLoad("metrics"),
  component: MetricsListRoute,
})

function MetricsListRoute() {
  return <ExploreList kind="metrics" />
}
