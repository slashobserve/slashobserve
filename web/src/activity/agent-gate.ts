import { maxQueuedRevisions } from "@/revisions/queries"

const provisionedReason =
  "This starter dashboard is kept current by SlashObserve releases, so the agent cannot change it. Investigate to ask a question, or duplicate it to get a copy the agent can change."

export type AgentGate =
  | { kind: "hidden" }
  | { kind: "allowed" }
  | { kind: "no-model" }
  | { kind: "blocked"; reason: string; provisioned?: true }

export function sendToAgentState(input: {
  canComment: boolean
  agentAvailable: boolean | undefined
  pinned: boolean
  layoutEditing: boolean
  activeRequests: number
  provisioned: boolean
}): AgentGate {
  if (!input.canComment) return { kind: "hidden" }
  if (input.agentAvailable === false) return { kind: "no-model" }
  if (input.pinned) {
    return {
      kind: "blocked",
      reason:
        "You are viewing an older version. Open the latest version to send changes to the agent.",
    }
  }
  if (input.layoutEditing) {
    return {
      kind: "blocked",
      reason: "Finish editing the layout to send changes to the agent.",
    }
  }
  if (input.activeRequests >= maxQueuedRevisions) {
    return {
      kind: "blocked",
      reason: `${maxQueuedRevisions} requests are already waiting for the agent. Send this when one finishes.`,
    }
  }
  if (input.provisioned) {
    return { kind: "blocked", reason: provisionedReason, provisioned: true }
  }
  return { kind: "allowed" }
}
