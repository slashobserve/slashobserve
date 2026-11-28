import { createFileRoute } from "@tanstack/react-router"

import { exploreEntryBeforeLoad } from "@/datasources/explore-entry"
import { ExploreList } from "@/datasources/explore-list"
import { validateTraceEntrySearch } from "@/traces/search"

export const Route = createFileRoute("/_authed/_shell/traces/")({
  validateSearch: validateTraceEntrySearch,
  staticData: { crumbs: [{ label: "Traces" }] },
  beforeLoad: exploreEntryBeforeLoad("traces"),
  component: TracesListRoute,
})

function TracesListRoute() {
  return <ExploreList kind="traces" />
}
