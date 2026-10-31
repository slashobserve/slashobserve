import { createFileRoute } from "@tanstack/react-router"

import { RoleNote } from "@/components/role-note"
import { useCanCreateDashboards } from "@/folders/create-access"
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
  const canCreate = useCanCreateDashboards()
  if (canCreate === undefined) return null
  if (!canCreate) {
    return (
      <RoleNote className="py-2">
        Importing needs Edit on a folder. Ask an Admin if you need it.
      </RoleNote>
    )
  }
  return (
    <div className="mx-auto w-full max-w-6xl">
      <ImportPage />
    </div>
  )
}
