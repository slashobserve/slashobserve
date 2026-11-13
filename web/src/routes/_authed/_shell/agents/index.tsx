import { createFileRoute } from "@tanstack/react-router"

import { AgentsPage } from "@/agents/agents-page"
import { useDocumentTitle } from "@/lib/document-title"

type AgentsSearch = { connect?: "1" }

export const Route = createFileRoute("/_authed/_shell/agents/")({
  staticData: { crumbs: [{ label: "Agents" }] },
  validateSearch: (search: Record<string, unknown>): AgentsSearch => ({
    connect: String(search.connect) === "1" ? "1" : undefined,
  }),
  component: AgentsRoute,
})

function AgentsRoute() {
  useDocumentTitle("Agents - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  return (
    <AgentsPage
      connecting={search.connect === "1"}
      onConnectingChange={(connecting) =>
        void navigate({
          search: connecting ? { connect: "1" } : {},
          replace: !connecting,
        })
      }
    />
  )
}
