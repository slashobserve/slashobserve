import { createFileRoute } from "@tanstack/react-router"

import { datasourcesQuery } from "@/datasources/queries"
import { agentInfoQuery } from "@/observe/queries"
import { validateTraceSearch } from "@/traces/search"
import { TracesPage } from "@/traces/traces-page"

export const Route = createFileRoute("/_authed/_shell/traces/$datasourceUid")({
  validateSearch: validateTraceSearch,
  staticData: { crumbs: [{ label: "Traces", to: "/traces" }], pageTitle: true },
  loader: async ({ context }) => {
    await Promise.allSettled([
      context.queryClient.ensureQueryData(datasourcesQuery()),
      context.queryClient.ensureQueryData(agentInfoQuery()),
    ])
  },
  component: TracesSourceRoute,
})

function TracesSourceRoute() {
  const { datasourceUid } = Route.useParams()
  return <TracesPage uid={datasourceUid} />
}
