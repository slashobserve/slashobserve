import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"

import { toApiError } from "@/api/errors"
import { agentKeys } from "@/api/keys"
import { agentName } from "@/connected-agents/rows"
import { useMyGrants } from "@/connected-agents/queries"
import { useAgentInfo } from "@/observe/queries"

import { observeAgent, type MentionAgent } from "./mentions"

type AgentDirectory = {
  service_accounts: readonly { id: string; name: string }[]
}

async function fetchAgentDirectory(signal: AbortSignal) {
  const response = await fetch("/api/v1/agents", {
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    signal,
  })
  const body: unknown = await response.json().catch(() => null)
  if (!response.ok) throw toApiError(response, body)
  return body as AgentDirectory
}

export function mentionableAgents(input: {
  observeAvailable: boolean
  grants: readonly { id: string; client: { name: string | null } }[]
  serviceAccounts: readonly { id: string; name: string }[]
}): MentionAgent[] {
  const agents = new Map<string, MentionAgent>()
  if (input.observeAvailable) agents.set("observe", observeAgent)
  for (const grant of input.grants) {
    agents.set(`grant:${grant.id}`, {
      kind: "grant",
      id: grant.id,
      name: agentName(grant.client.name),
    })
  }
  for (const account of input.serviceAccounts) {
    agents.set(`service_account:${account.id}`, {
      kind: "service_account",
      id: account.id,
      name: account.name,
    })
  }
  return [...agents.values()]
}

export function useMentionableAgents(): MentionAgent[] {
  const info = useAgentInfo()
  const grants = useMyGrants()
  const directory = useQuery({
    queryKey: agentKeys.list(),
    queryFn: ({ signal }) => fetchAgentDirectory(signal),
    retry: false,
    staleTime: 60_000,
  })
  const available = info.data?.available === true
  const grantList = grants.data
  const serviceAccounts = directory.data?.service_accounts
  return useMemo(
    () =>
      mentionableAgents({
        observeAvailable: available,
        grants: grantList ?? [],
        serviceAccounts: serviceAccounts ?? [],
      }),
    [available, grantList, serviceAccounts]
  )
}
