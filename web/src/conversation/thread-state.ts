import type { Message, Thread } from "@/comments/thread-queries"

import type { AgentGate } from "./delivery"

export type Run = NonNullable<Message["run"]>

export type ThreadStatus = { kind: "resolved" | "open"; text: string }

export function threadStatus(
  thread: Pick<Thread, "status" | "resolved_by_version" | "resolved_actor">,
  gate: AgentGate,
  clientName: string
): ThreadStatus {
  if (thread.status === "resolved") {
    return {
      kind: "resolved",
      text:
        thread.resolved_by_version === null
          ? `Resolved by ${thread.resolved_actor?.label ?? "someone"}`
          : `Resolved by version ${thread.resolved_by_version}`,
    }
  }
  return {
    kind: "open",
    text: gate.kind === "no-model" ? `Open, waiting for ${clientName}` : "Open",
  }
}

export function panelText(
  panelId: string | null,
  panelTitle: string | null
): string | undefined {
  if (panelId === null) return undefined
  return panelTitle === null ? "on a removed panel" : "on"
}

export function unansweredRun(
  message: Pick<Message, "run">,
  messages: readonly Pick<Message, "actor">[]
): Run | null {
  const run = message.run ?? null
  if (run === null) return null
  return messages.some((reply) => reply.actor.run_id === run.id) ? null : run
}

export function askedAgentName(
  message: Pick<Message, "mentions">,
  run: Pick<Run, "assigned_kind">
): string {
  if (run.assigned_kind === "builtin") return "Observe Agent"
  const named = message.mentions.find(
    (mention) => mention.kind === run.assigned_kind
  )
  return named?.name ?? "The agent"
}

export type RunProgress = Run["status"] | "canceling" | "canceled"

export function runProgress(
  request: { status: Run["status"]; state?: string } | undefined,
  sent: Run["status"]
): RunProgress {
  if (request?.state === "canceling" || request?.state === "canceled") {
    return request.state
  }
  return request?.status ?? sent
}

export function runLineText(
  run: Pick<Run, "assigned_kind">,
  progress: RunProgress,
  agentName: string
): string {
  if (run.assigned_kind !== "builtin") return `${agentName} was asked`
  switch (progress) {
    case "canceling":
      return "Stopping"
    case "canceled":
      return "Stopped"
    case "queued":
    case "running":
      return `${agentName} is working on it`
    case "succeeded":
      return `${agentName} finished`
    case "failed":
      return `${agentName} could not finish`
  }
}

export function replyCountText(count: number): string {
  return count === 1 ? "1 reply" : `${count} replies`
}
