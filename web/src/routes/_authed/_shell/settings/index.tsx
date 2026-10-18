import { useQuery } from "@tanstack/react-query"
import { createFileRoute } from "@tanstack/react-router"

import { instanceQuery } from "@/api/instance"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import { useDocumentTitle } from "@/lib/document-title"
import { useTheme, type ThemePreference } from "@/lib/theme"
import { useAgentInfo } from "@/observe/queries"
import { domainList } from "@/people/sign-in-policy"
import { loadProblemText } from "@/service-accounts/queries"
import {
  domainsText,
  effortName,
  providerName,
  sessionLength,
} from "@/settings/format"
import {
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "@/settings/settings-card"

export const Route = createFileRoute("/_authed/_shell/settings/")({
  staticData: { crumbs: [{ label: "Settings" }] },
  component: SettingsPage,
})

const themeItems: { value: ThemePreference; label: string }[] = [
  { value: "system", label: "Match the system" },
  { value: "light", label: "Light" },
  { value: "dark", label: "Dark" },
]

function Pending() {
  return <Skeleton className="h-4 w-32" />
}

function SettingsPage() {
  useDocumentTitle("Settings - SlashObserve")
  return (
    <div className="mx-auto w-full max-w-5xl">
      <PageHeader
        title="Settings"
        description="Read from the server environment. Change them in SLASHOBSERVE_* settings and restart."
      />
      <AgentCard />
      <ServerCard />
      <SignInCard />
      <AppearanceCard />
    </div>
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
      <SettingsRow label="Public URL" mono copy={instance?.public_url}>
        {instance ? instance.public_url : <Pending />}
      </SettingsRow>
      <SettingsRow label="MCP URL" mono copy={instance?.mcp_url}>
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

function AppearanceCard() {
  const { preference, setPreference } = useTheme()
  return (
    <SettingsCard title="Appearance" hint="Saved in this browser only.">
      <SettingsRow label="Theme">
        <Select
          items={themeItems}
          value={preference}
          onValueChange={(next) => next && setPreference(next)}
        >
          <SelectTrigger aria-label="Theme" className="w-56">
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false} align="start">
            {themeItems.map((item) => (
              <SelectItem key={item.value} value={item.value}>
                {item.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </SettingsRow>
    </SettingsCard>
  )
}
