import { createFileRoute } from "@tanstack/react-router"

import { datasourcesQuery } from "@/datasources/queries"
import { agentInfoQuery } from "@/observe/queries"
import { LogsPage } from "@/logs/logs-page"
import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/$datasourceUid")({
  validateSearch: validateLogsSearch,
  staticData: { crumbs: [{ label: "Logs", to: "/logs" }], pageTitle: true },
  loader: async ({ context }) => {
    await Promise.allSettled([
      context.queryClient.ensureQueryData(datasourcesQuery()),
      context.queryClient.ensureQueryData(agentInfoQuery()),
    ])
  },
  component: LogsSourceRoute,
})

function LogsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <LogsPage uid={datasourceUid} />
}
