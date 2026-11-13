import { createFileRoute } from "@tanstack/react-router"

import { LogsPage } from "@/logs/logs-page"
import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/$datasourceUid")({
  validateSearch: validateLogsSearch,
  staticData: { crumbs: [{ label: "Logs", to: "/logs" }], pageTitle: true },
  component: LogsSourceRoute,
})

function LogsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <LogsPage uid={datasourceUid} />
}
