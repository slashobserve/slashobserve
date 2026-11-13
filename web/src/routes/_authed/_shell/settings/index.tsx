import {
  createFileRoute,
  useLocation,
  useRouteContext,
} from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { ConnectedAgentsSection } from "@/connected-agents/connected-agents-card"
import { useDocumentTitle } from "@/lib/document-title"
import { roleAtLeast } from "@/lib/roles"
import { AgentAccessSection } from "@/settings/agent-access-section"
import { AppearanceSection } from "@/settings/appearance-card"
import { GrafanaSettings } from "@/settings/grafana/grafana-settings"
import { OutboundAccessSection } from "@/settings/outbound-access-card"
import {
  selectedSectionId,
  settingsGroups,
  settingsPanel,
  type SettingsPanel,
  type SettingsSectionId,
} from "@/settings/sections"
import { ServerSection } from "@/settings/server-section"
import { SettingsNav, SettingsSectionSelect } from "@/settings/settings-nav"
import { SignInSection } from "@/settings/sign-in-section"
import { WebhooksSettings } from "@/settings/webhooks/webhooks-settings"

type SettingsSearch = {
  connection?: string
  sync?: string
  panel?: string
  webhook?: string
}

function optionalText(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

export const Route = createFileRoute("/_authed/_shell/settings/")({
  staticData: { crumbs: [{ label: "Settings" }] },
  validateSearch: (search: Record<string, unknown>): SettingsSearch => ({
    connection: optionalText(search.connection),
    sync: optionalText(search.sync),
    panel: optionalText(search.panel),
    webhook: optionalText(search.webhook),
  }),
  component: SettingsPage,
})

function SettingsPage() {
  useDocumentTitle("Settings - SlashObserve")
  const { principal } = useRouteContext({ from: "/_authed" })
  const hash = useLocation({ select: (location) => location.hash })
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const groups = settingsGroups(principal.role)
  const selected = selectedSectionId(groups, hash)
  const panel = settingsPanel(selected, search)

  function onPanel(next: SettingsPanel | null) {
    void navigate({
      hash: selected,
      search: (prev) => ({
        ...prev,
        panel: next?.kind,
        webhook: next?.kind === "webhook-edit" ? next.webhookId : undefined,
      }),
      replace: true,
    })
  }

  return (
    <>
      <PageHeader title="Settings" />
      <div className="flex items-start gap-8">
        <SettingsNav
          groups={groups}
          selected={selected}
          member={!roleAtLeast(principal.role, "admin")}
        />
        <div className="flex min-w-0 flex-1 flex-col gap-4">
          <SettingsSectionSelect groups={groups} selected={selected} />
          <SelectedSection id={selected} panel={panel} onPanel={onPanel} />
        </div>
      </div>
    </>
  )
}

function SelectedSection({
  id,
  panel,
  onPanel,
}: {
  id: SettingsSectionId
  panel: SettingsPanel | null
  onPanel: (panel: SettingsPanel | null) => void
}) {
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  switch (id) {
    case "appearance":
      return <AppearanceSection />
    case "connected-agents":
      return <ConnectedAgentsSection />
    case "server":
      return <ServerSection />
    case "sign-in":
      return <SignInSection />
    case "outbound-access":
      return <OutboundAccessSection />
    case "agent-access":
      return <AgentAccessSection />
    case "grafana":
      return (
        <GrafanaSettings
          running={
            search.connection !== undefined && search.sync !== undefined
              ? { connectionId: search.connection, syncId: search.sync }
              : null
          }
          onStarted={(connectionId, syncId) =>
            void navigate({
              search: {
                connection: connectionId,
                sync: syncId,
                panel: undefined,
                webhook: undefined,
              },
              hash: "grafana" satisfies SettingsSectionId,
            })
          }
          panelOpen={panel?.kind === "grafana-add"}
          onPanel={onPanel}
        />
      )
    case "webhooks":
      return <WebhooksSettings panel={panel} onPanel={onPanel} />
  }
}
