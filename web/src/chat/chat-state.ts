import type { StatusTone } from "@/components/status-label"
import { buildStatusText } from "@/observe/build"
import { requestStatusView, type RequestSummary } from "@/observe/queries"

export function chatState(state: RequestSummary["state"]): {
  word: "Running" | "Stopped" | "Done"
  tone: StatusTone
} {
  switch (state) {
    case "queued":
    case "running":
    case "canceling":
      return { word: "Running", tone: "running" }
    case "succeeded":
      return { word: "Done", tone: "ok" }
    case "failed":
    case "canceled":
      return { word: "Stopped", tone: "warning" }
  }
}

export type ChatStateFilter = "running" | "stopped" | "done"

const everyState = {
  queued: true,
  running: true,
  canceling: true,
  succeeded: true,
  failed: true,
  canceled: true,
} satisfies Record<RequestSummary["state"], true>

export function statesFor(filter: ChatStateFilter): RequestSummary["state"][] {
  return (Object.keys(everyState) as RequestSummary["state"][]).filter(
    (state) => chatState(state).word.toLowerCase() === filter
  )
}

function savedText(request: RequestSummary): string {
  if (request.dashboard_title === null) return requestStatusView(request).text
  return request.source === "observe"
    ? `Saved ${request.dashboard_title}`
    : `Revised ${request.dashboard_title}`
}

function observationOutcome(request: RequestSummary): string {
  if (request.kind === "build") {
    return request.dashboard_id === null
      ? "Answered, no dashboard"
      : "Answered, dashboard unchanged"
  }
  return request.observation_title === null
    ? "Saved an observation"
    : `Answered in ${request.observation_title}`
}

function succeededOutcome(request: RequestSummary): string {
  switch (request.result_kind) {
    case "version":
    case "partial_version":
      return savedText(request)
    case "observation":
      return observationOutcome(request)
    case "suggestion":
      return "Suggested a panel"
    case "proposal":
      return "Opened a proposal"
    case "none":
      return "Nothing saved"
  }
}

export function deleteChatText(request: RequestSummary): string {
  const left = "It leaves your chat lists and cannot be added back."
  switch (request.result_kind) {
    case "version":
    case "partial_version":
      return `${left} The dashboard it made stays.`
    case "observation":
      if (request.observation_id === null) {
        return `${left} The answer stays on the chat page, which still opens from its link.`
      }
      return request.observation_title === null
        ? `${left} The answer stays in its observation, which is not deleted.`
        : `${left} The answer stays in the observation "${request.observation_title}", which is not deleted.`
    case "suggestion":
      return `${left} The panel suggestion stays.`
    case "proposal":
      return `${left} The proposal stays.`
    case "none":
      return `${left} Nothing else is removed.`
  }
}

export function deleteTurnsText(turns: readonly RequestSummary[]): string {
  if (turns.length === 1) return deleteChatText(turns[0])
  return `All ${turns.length} messages in this chat leave your chat lists and cannot be added back. What they saved, such as dashboards, observations and proposals, stays.`
}

export function chatOutcome(request: RequestSummary): string {
  switch (request.state) {
    case "queued":
    case "running":
    case "canceling":
      return buildStatusText({
        status: request.status,
        build: request.build,
        failureCode: request.failure_code,
        canceling: request.state === "canceling",
      })
    case "failed":
    case "canceled":
      if (request.result_kind === "partial_version") {
        return request.dashboard_title === null
          ? requestStatusView(request).text
          : `Saved part of ${request.dashboard_title}`
      }
      return buildStatusText({
        status: request.status,
        build: request.build,
        failureCode: request.failure_code,
        continued: request.continued_by !== null,
      })
    case "succeeded":
      return succeededOutcome(request)
  }
}
