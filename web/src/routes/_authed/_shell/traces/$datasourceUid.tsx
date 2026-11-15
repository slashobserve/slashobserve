import { createFileRoute } from "@tanstack/react-router"

import { validateTraceSearch } from "@/traces/search"
import { TracesPage } from "@/traces/traces-page"

export const Route = createFileRoute("/_authed/_shell/traces/$datasourceUid")({
  validateSearch: validateTraceSearch,
  staticData: { crumbs: [{ label: "Traces", to: "/traces" }], pageTitle: true },
  component: TracesSourceRoute,
})

function TracesSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <TracesPage uid={datasourceUid} />
}
