import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  createFileRoute,
  useLocation,
  useRouteContext,
} from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { useAgentInfo } from "@/observe/queries"
import { domainList } from "@/people/sign-in-policy"
import { loadProblemText } from "@/service-accounts/queries"
import {
  domainsText,
  effortName,
  providerName,
  sessionLength,
} from "@/settings/format"
import { GrafanaSettings } from "@/settings/grafana/grafana-settings"
import { OutboundAccessCard } from "@/settings/outbound-access-card"
import {
  selectedSectionId,
  settingsSections,
  type SettingsSectionId,
} from "@/settings/sections"
import {
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "@/settings/settings-card"
import { SettingsNav } from "@/settings/settings-nav"

type SettingsSearch = { connection?: string; sync?: string }

function optionalText(value: unknown): string | undefined {
  return typeof value === "string" && value !== "" ? value : undefined
}

export const Route = createFileRoute("/_authed/_shell/settings/")({
  staticData: { crumbs: [{ label: "Settings" }] },
  validateSearch: (search: Record<string, unknown>): SettingsSearch => ({
    connection: optionalText(search.connection),
    sync: optionalText(search.sync),
  }),
  component: SettingsPage,
})

function Pending() {
  return <Skeleton className="h-4 w-32" />
}

function SettingsPage() {
  useDocumentTitle("Settings - SlashObserve")
  const { principal } = useRouteContext({ from: "/_authed" })
  const hash = useLocation({ select: (location) => location.hash })
  const sections = settingsSections(principal.role)
  const selected = selectedSectionId(sections, hash)
  return (
    <div className="flex items-start gap-8">
      <SettingsNav sections={sections} selected={selected} placement="side" />
      <div className="max-w-[720px] min-w-0 flex-1">
        <PageHeader
          title="Settings"
          description="How this server is set up. Most values come from its environment."
        />
        <SettingsNav sections={sections} selected={selected} placement="top" />
        <SelectedSection id={selected} />
      </div>
    </div>
  )
}

function SelectedSection({ id }: { id: SettingsSectionId }) {
  switch (id) {
    case "agent":
      return <AgentCard />
    case "server":
      return <ServerCard />
    case "sign-in":
      return <SignInCard />
    case "outbound-access":
      return <OutboundAccessCard />
    case "grafana":
      return <GrafanaSection />
  }
}

function GrafanaSection() {
  const headingId = useId()
  const search = Route.useSearch()
  const navigate = Route.useNavigate()
  const running =
    search.connection !== undefined && search.sync !== undefined
      ? { connectionId: search.connection, syncId: search.sync }
      : null
  return (
    <section aria-labelledby={headingId}>
      <div className="mb-3">
        <h2 id={headingId} className="text-sm font-semibold">
          Grafana connection
        </h2>
        <p className="mt-0.5 text-xs text-muted-foreground">
          Connect a Grafana server and import its dashboards with their folders.
        </p>
      </div>
      <GrafanaSettings
        running={running}
        onStarted={(connectionId, syncId) =>
          void navigate({
            search: { connection: connectionId, sync: syncId },
            hash: "grafana" satisfies SettingsSectionId,
          })
        }
      />
    </section>
  )
}

function AgentCard() {
  const query = useAgentInfo()
  const hint = "The model that builds and revises dashboards."

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="Agent" hint={hint}>
        <SettingsProblem
          title="Could not load the agent settings"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }

  const agent = query.data
  return (
    <SettingsCard title="Agent" hint={hint}>
      <SettingsRow label="Provider">
        {agent ? providerName(agent.provider) : <Pending />}
      </SettingsRow>
      <SettingsRow label="Model" mono={agent?.model != null}>
        {agent ? (agent.model ?? "Not set") : <Pending />}
      </SettingsRow>
      <SettingsRow label="Effort">
        {agent ? (
          agent.available ? (
            effortName(agent.effort)
          ) : (
            "Not set"
          )
        ) : (
          <Pending />
        )}
      </SettingsRow>
      <SettingsRow label="Status">
        {agent ? (
          agent.available ? (
            <StatusLabel tone="ok">Connected</StatusLabel>
          ) : (
            <StatusLabel tone="warning">
              Not configured. Set SLASHOBSERVE_LLM_PROVIDER and restart.
            </StatusLabel>
          )
        ) : (
          <Pending />
        )}
      </SettingsRow>
    </SettingsCard>
  )
}

function ServerCard() {
  const query = useQuery(instanceQuery())
  const hint = "Addresses clients use to reach it."

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="This server" hint={hint}>
        <SettingsProblem
          title="Could not load the server settings"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }

  const instance = query.data
  return (
    <SettingsCard title="This server" hint={hint}>
      <SettingsRow label="Public URL" mono copy={instance?.public_url ?? null}>
        {instance ? instance.public_url : <Pending />}
      </SettingsRow>
      <SettingsRow label="MCP URL" mono copy={instance?.mcp_url ?? null}>
        {instance ? instance.mcp_url : <Pending />}
      </SettingsRow>
    </SettingsCard>
  )
}

function SignInCard() {
  const query = useQuery(instanceQuery())
  const hint = "Who can sign in, through your identity provider."

  if (query.isError && query.data === undefined) {
    return (
      <SettingsCard title="Sign-in" hint={hint}>
        <SettingsProblem
          title="Could not load the sign-in settings"
          detail={loadProblemText(query.error)}
          retrying={query.isFetching}
          onRetry={() => void query.refetch()}
        />
      </SettingsCard>
    )
  }

  const instance = query.data
  return (
    <SettingsCard title="Sign-in" hint={hint}>
      <SettingsRow label="Identity provider" mono>
        {instance ? instance.oidc_issuer_url : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Allowed email domains"
        mono={
          instance !== undefined &&
          !instance.any_email_domain &&
          domainList(instance.allowed_email_domains).length > 0
        }
      >
        {instance ? domainsText(instance) : <Pending />}
      </SettingsRow>
      <SettingsRow label="Session length">
        {instance ? sessionLength(instance.session_ttl_seconds) : <Pending />}
      </SettingsRow>
    </SettingsCard>
  )
}
