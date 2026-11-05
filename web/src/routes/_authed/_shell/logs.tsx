import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import { LogsPage } from "@/logs/logs-page"
import { validateLogsSearch } from "@/logs/search"

export const Route = createFileRoute("/_authed/_shell/logs")({
  validateSearch: validateLogsSearch,
  staticData: { crumbs: [{ label: "Logs" }] },
  component: LogsRoute,
})

function LogsRoute() {
  useDocumentTitle("Logs - SlashObserve")
  return <LogsPage />
}
