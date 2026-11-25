import { createFileRoute } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { DatasourcesPage } from "@/datasources/datasources-page"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute(
  "/_authed/_shell/datasources/context-sources"
)({
  staticData: {
    crumbs: [
      { label: "Datasources", to: "/datasources" },
      { label: "MCP sources" },
    ],
  },
  component: ContextSourcesRoute,
})

function ContextSourcesRoute() {
  useDocumentTitle("MCP sources - SlashObserve")
  return (
    <RoleGate min="admin">
      <DatasourcesPage focusMcpSources />
    </RoleGate>
  )
}
