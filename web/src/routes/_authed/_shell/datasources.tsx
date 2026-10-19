import { createFileRoute } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/datasources")({
  staticData: { crumbs: [{ label: "Datasources" }] },
  component: DatasourcesPage,
})

function DatasourcesPage() {
  useDocumentTitle("Datasources - SlashObserve")
  return (
    <>
      <PageHeader
        title="Datasources"
        description="Defined in the datasources file on the server. Edit that file and restart to add or change one."
      />
    </>
  )
}
