import { useQuery } from "@tanstack/react-query"

import { instanceQuery } from "@/api/instance"
import { StatusLabel } from "@/components/status-label"
import { Skeleton } from "@/components/ui/skeleton"
import { useAgentInfo } from "@/observe/queries"
import { loadProblemText } from "@/service-accounts/queries"

import { effortName, providerName } from "./format"
import {
  EnvChip,
  SettingsCard,
  SettingsProblem,
  SettingsRow,
} from "./settings-card"
import { SettingsSection } from "./settings-section"

function Pending() {
  return <Skeleton className="h-4 w-32" />
}

export function ServerSection() {
  return (
    <SettingsSection
      title="Server"
      description="These values are read from the server's environment. Change them there and restart."
    >
      <ThisServerCard />
      <AgentCard />
    </SettingsSection>
  )
}

function ThisServerCard() {
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
      <SettingsRow
        label="Public URL"
        mono
        copy={instance?.public_url ?? null}
        note={<EnvChip name="SLASHOBSERVE_PUBLIC_URL" />}
      >
        {instance ? instance.public_url : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="MCP URL"
        mono
        copy={instance?.mcp_url ?? null}
        note={
          <span className="text-xs text-muted-foreground">
            Public URL plus /mcp
          </span>
        }
      >
        {instance ? instance.mcp_url : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Version"
        mono
        note={<span className="text-xs text-muted-foreground">This build</span>}
      >
        {instance ? instance.version : <Pending />}
      </SettingsRow>
    </SettingsCard>
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
      <SettingsRow
        label="Provider"
        note={<EnvChip name="SLASHOBSERVE_LLM_PROVIDER" />}
      >
        {agent ? providerName(agent.provider) : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Model"
        mono={agent?.model != null}
        note={<EnvChip name="SLASHOBSERVE_LLM_MODEL" />}
      >
        {agent ? (agent.model ?? "Not set") : <Pending />}
      </SettingsRow>
      <SettingsRow
        label="Effort"
        note={<EnvChip name="SLASHOBSERVE_LLM_EFFORT" />}
      >
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
