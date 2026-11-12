import { useQuery } from "@tanstack/react-query"
import { useMemo } from "react"

import { agentsQuery } from "@/agents/queries"
import { useAgentInfo } from "@/observe/queries"

import { observeAgent, type MentionAgent } from "./mentions"

export function mentionableAgents(input: {
  observeAvailable: boolean
  connected: readonly { grant_id: string; client_name: string }[]
  serviceAccounts: readonly { id: string; name: string }[]
}): MentionAgent[] {
  const agents = new Map<string, MentionAgent>()
  if (input.observeAvailable) agents.set("observe", observeAgent)
  for (const grant of input.connected) {
    agents.set(`grant:${grant.grant_id}`, {
      kind: "grant",
      id: grant.grant_id,
      name: grant.client_name,
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
  const agents = useQuery(agentsQuery())
  const available = info.data?.available === true
  const connected = agents.data?.connected
  const serviceAccounts = agents.data?.service_accounts
  return useMemo(
    () =>
      mentionableAgents({
        observeAvailable: available,
        connected: connected ?? [],
        serviceAccounts: serviceAccounts ?? [],
      }),
    [available, connected, serviceAccounts]
  )
}
