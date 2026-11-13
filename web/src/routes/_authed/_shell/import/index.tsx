import { useEffect, useState } from "react"
import { createFileRoute } from "@tanstack/react-router"

import { useDocumentTitle } from "@/lib/document-title"
import { clearHandedImport, peekHandedImport } from "@/import/handoff"
import { ImportPage } from "@/import/import-page"

export const Route = createFileRoute("/_authed/_shell/import/")({
  staticData: {
    crumbs: [{ label: "Home", to: "/" }, { label: "Import from Grafana" }],
  },
  component: ImportRoute,
})

function ImportRoute() {
  useDocumentTitle("Import from Grafana - SlashObserve")
  const [initial] = useState(peekHandedImport)
  useEffect(() => clearHandedImport(), [])
  return <ImportPage initial={initial} />
}
