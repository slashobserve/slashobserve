import { createFileRoute } from "@tanstack/react-router"

import { LogsPage } from "@/logs/logs-page"
import { preloadLogsSource } from "@/logs/preload-source"
import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/$datasourceUid")({
  validateSearch: validateLogsSearch,
  staticData: { crumbs: [{ label: "Logs", to: "/logs" }], pageTitle: true },
  loader: ({ context }) => preloadLogsSource(context.queryClient),
  component: LogsSourceRoute,
})

function LogsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <LogsPage uid={datasourceUid} />
}
