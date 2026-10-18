import { createFileRoute } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { useDocumentTitle } from "@/lib/document-title"

export const Route = createFileRoute("/_authed/_shell/settings/")({
  staticData: { crumbs: [{ label: "Settings" }] },
  component: SettingsPage,
})

function SettingsPage() {
  useDocumentTitle("Settings - SlashObserve")
  return (
    <div className="mx-auto w-full max-w-5xl">
      <PageHeader
        title="Settings"
        description="Read from the server environment. Change them in SLASHOBSERVE_* settings and restart."
      />
    </div>
  )
}
