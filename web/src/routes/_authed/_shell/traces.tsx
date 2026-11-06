import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import { validateTraceSearch } from "@/traces/search"
import { TracesPage } from "@/traces/traces-page"

export const Route = createFileRoute("/_authed/_shell/traces")({
  validateSearch: validateTraceSearch,
  staticData: { crumbs: [{ label: "Traces" }] },
  component: TracesRoute,
})

function TracesRoute() {
  useDocumentTitle("Traces - SlashObserve")
  return <TracesPage />
}
