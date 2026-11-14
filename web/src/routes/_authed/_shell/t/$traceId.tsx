import { createFileRoute } from "@tanstack/react-router"

import { validateTraceViewSearch } from "@/traces/search"
import { TracePage } from "@/traces/trace-page"

export const Route = createFileRoute("/_authed/_shell/t/$traceId")({
  validateSearch: validateTraceViewSearch,
  staticData: {
    crumbs: [{ label: "Traces", to: "/traces" }],
    pageTitle: true,
  },
  component: TraceRoute,
})

function TraceRoute() {
  const { traceId } = Route.useParams()
  return <TracePage traceId={traceId} />
}
