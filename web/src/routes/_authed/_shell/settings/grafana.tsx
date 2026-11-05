import { createFileRoute } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { RoleGate } from "@/components/role-gate"
import { useDocumentTitle } from "@/lib/document-title"
import { GrafanaSettings } from "@/settings/grafana/grafana-settings"
import { SettingsTabs } from "@/settings/settings-tabs"

type GrafanaSearch = { connection?: string; sync?: string }

function optionalText(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

export const Route = createFileRoute("/_authed/_shell/settings/grafana")({
  staticData: {
    crumbs: [{ label: "Settings", to: "/settings" }, { label: "Grafana" }],
  },
  validateSearch: (search: Record<string, unknown>): GrafanaSearch => ({
    connection: optionalText(search.connection),
    sync: optionalText(search.sync),
  }),
  component: GrafanaRoute,
})

function GrafanaRoute() {
  useDocumentTitle("Grafana - SlashObserve")
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const running =
    search.connection !== undefined && search.sync !== undefined
      ? { connectionId: search.connection, syncId: search.sync }
      : null
  return (
    <RoleGate min="admin">
      <PageHeader
        title="Grafana"
        description="Connect a Grafana server and import its dashboards with their folders."
      />
      <SettingsTabs />
      <GrafanaSettings
        running={running}
        onStarted={(connectionId, syncId) =>
          void navigate({ search: { connection: connectionId, sync: syncId } })
        }
      />
    </RoleGate>
  )
}
