import { createFileRoute } from "@tanstack/react-router"

import { RoleGate } from "@/components/role-gate"
import { DatasourcesPage } from "@/datasources/datasources-page"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/datasources/")({
  staticData: { crumbs: [{ label: "Datasources" }] },
  component: DatasourcesRoute,
})

function DatasourcesRoute() {
  useDocumentTitle("Datasources - SlashObserve")
  return (
    <RoleGate min="admin">
      <DatasourcesPage />
    </RoleGate>
  )
}
