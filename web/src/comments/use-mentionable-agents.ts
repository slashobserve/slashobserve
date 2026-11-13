import { useMemo } from "react"

import { agentName } from "@/connected-agents/rows"
import { useMyGrants } from "@/connected-agents/queries"
import { useAgentInfo } from "@/observe/queries"

import { observeAgent, type MentionAgent } from "./mentions"

export function mentionableAgents(input: {
  observeAvailable: boolean
  grants: readonly { id: string; client: { name: string | null } }[]
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
  return [...agents.values()]
}

export function useMentionableAgents(): MentionAgent[] {
  const info = useAgentInfo()
  const grants = useMyGrants()
  const available = info.data?.available === true
  const grantList = grants.data
  return useMemo(
    () =>
      mentionableAgents({
        observeAvailable: available,
        grants: grantList ?? [],
      }),
    [available, grantList]
  )
}
