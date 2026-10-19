import type { Comment } from "@/comments/queries"
import { isActive, type Revision } from "@/revisions/queries"

import type { AgentGate } from "./agent-gate"

export type ThreadState =
  | { kind: "sent" }
  | { kind: "failed"; error: string | null }
  | { kind: "resolved"; text: string }
  | { kind: "note"; text: string }

export function threadVerbText(
  askedAgent: boolean,
  panelId: string | null,
  panelTitle: string | null
): string {
  const base = askedAgent ? "asked the agent" : "commented"
  if (panelId === null) return base
  if (panelTitle === null) return `${base} on a removed panel`
  return `${base} on`
}

export function threadState(input: {
  root: Comment
  request: Revision | null
  gate: AgentGate
  clientName: string
}): ThreadState {
  const { root, request, gate, clientName } = input
  const resolved = root.status === "resolved"
  if (request !== null && isActive(request.status)) return { kind: "sent" }
  if (request?.status === "failed" && !resolved) {
    return { kind: "failed", error: request.error }
  }
  if (resolved && root.resolved_by_version !== null) {
    return {
      kind: "resolved",
      text: `Resolved by version ${root.resolved_by_version}`,
    }
  }
  if (resolved) {
    return {
      kind: "resolved",
      text: `Resolved by ${root.resolved_by?.name ?? "someone"}`,
    }
  }
  if (request?.status === "succeeded" && request.result_version === null) {
    return { kind: "note", text: "Finished, nothing changed" }
  }
  if (gate.kind === "no-model") {
    return { kind: "note", text: `Open, waiting for ${clientName}` }
  }
  return { kind: "note", text: "Comment" }
}

export function canResolveThread(request: Revision | null): boolean {
  return request === null || !isActive(request.status)
}
