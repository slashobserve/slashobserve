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

function succeededOutcome(request: RequestSummary): string {
  switch (request.result_kind) {
    case "version":
    case "partial_version":
      return savedText(request)
    case "observation":
      return "Saved an observation"
    case "suggestion":
      return "Suggested a panel"
    case "proposal":
      return "Opened a proposal"
    case "none":
      return "Nothing saved"
  }
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
