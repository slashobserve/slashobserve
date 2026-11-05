import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import { validateTraceViewSearch } from "@/traces/search"
import { TracePage } from "@/traces/trace-page"

export const Route = createFileRoute("/_authed/_shell/t/$traceId")({
  validateSearch: validateTraceViewSearch,
  staticData: {
    crumbs: [{ label: "Traces", to: "/traces" }, { label: "Trace" }],
  },
  component: TraceRoute,
})

function TraceRoute() {
  const { traceId } = Route.useParams()
  useDocumentTitle(`Trace ${traceId.slice(0, 8)} - SlashObserve`)
  return <TracePage traceId={traceId} />
}
