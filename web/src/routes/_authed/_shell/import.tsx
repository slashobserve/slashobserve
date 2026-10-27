import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import { ImportPage } from "@/import/import-page"

export const Route = createFileRoute("/_authed/_shell/import")({
  staticData: {
    crumbs: [{ label: "Observe", to: "/" }, { label: "Import from Grafana" }],
  },
  component: ImportRoute,
})

function ImportRoute() {
  useDocumentTitle("Import from Grafana - SlashObserve")
  return (
    <div className="mx-auto w-full max-w-6xl">
      <ImportPage />
    </div>
  )
}
