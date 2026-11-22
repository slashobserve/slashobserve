import { createFileRoute } from "@tanstack/react-router"

import {
  LogsSource,
  logsSourceCrumbs,
  preloadLogsSource,
} from "@/logs/source-route"
import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs/$datasourceUid")({
  validateSearch: validateLogsSearch,
  staticData: { crumbs: logsSourceCrumbs, pageTitle: true },
  loader: ({ context, params }) =>
    preloadLogsSource(
      context.queryClient,
      context.principal.role,
      params.datasourceUid
    ),
  component: LogsSourceRoute,
})

function LogsSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <LogsSource uid={datasourceUid} />
}
