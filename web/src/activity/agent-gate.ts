import { maxQueuedRevisions } from "@/revisions/queries"

export type AgentGate =
  | { kind: "hidden" }
  | { kind: "allowed" }
  | { kind: "no-model" }
  | { kind: "blocked"; reason: string }

export function sendToAgentState(input: {
  canEdit: boolean
  agentAvailable: boolean | undefined
  pinned: boolean
  layoutEditing: boolean
  activeRequests: number
}): AgentGate {
  if (!input.canEdit) return { kind: "hidden" }
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
  return { kind: "allowed" }
}
