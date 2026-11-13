import type { AgentsOverview, ConnectedRow, ServiceAccountRow } from "./queries"

export type AgentRef =
  | { kind: "observe" }
  | { kind: "grant"; id: string }
  | { kind: "service_account"; id: string }

export type FoundAgent =
  | { kind: "observe"; row: AgentsOverview["observe_agent"] }
  | { kind: "grant"; row: ConnectedRow }
  | { kind: "service_account"; row: ServiceAccountRow }

const grantPrefix = "grant-"
const accountPrefix = "sa-"
const idPattern = /^[A-Za-z0-9_-]{1,64}$/

export function agentRefPath(ref: AgentRef): string {
  switch (ref.kind) {
    case "observe":
      return "observe"
    case "grant":
      return `${grantPrefix}${ref.id}`
    case "service_account":
      return `${accountPrefix}${ref.id}`
  }
}

export function agentLink(ref: AgentRef) {
  return {
    to: "/agents/$agentRef",
    params: { agentRef: agentRefPath(ref) },
  } as const
}

export function parseAgentRef(text: string): AgentRef | null {
  if (text === "observe") return { kind: "observe" }
  if (text.startsWith(grantPrefix)) {
    const id = text.slice(grantPrefix.length)
    return idPattern.test(id) ? { kind: "grant", id } : null
  }
  if (text.startsWith(accountPrefix)) {
    const id = text.slice(accountPrefix.length)
    return idPattern.test(id) ? { kind: "service_account", id } : null
  }
  return null
}

export function findAgent(
  overview: AgentsOverview,
  ref: AgentRef
): FoundAgent | null {
  switch (ref.kind) {
    case "observe":
      return { kind: "observe", row: overview.observe_agent }
    case "grant": {
      const row = overview.connected.find((item) => item.grant_id === ref.id)
      return row === undefined ? null : { kind: "grant", row }
    }
    case "service_account": {
      const row = overview.service_accounts.find((item) => item.id === ref.id)
      return row === undefined ? null : { kind: "service_account", row }
    }
  }
}
