import { createFileRoute } from "@tanstack/react-router"

import { exploreEntryBeforeLoad } from "@/datasources/explore-entry"
import { ExploreList } from "@/datasources/explore-list"
import { validateLogsEntrySearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/")({
  validateSearch: validateLogsEntrySearch,
  staticData: { crumbs: [{ label: "Logs" }] },
  beforeLoad: exploreEntryBeforeLoad("logs"),
  component: LogsListRoute,
})

function LogsListRoute() {
  return <ExploreList kind="logs" />
}
