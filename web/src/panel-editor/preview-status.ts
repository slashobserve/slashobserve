import { ApiError, failedQueryText } from "@/api/errors"
import type { WireFrame } from "@/dashboards/queries"
import { formatShortRelative } from "@/lib/time"
import { lowerFirst } from "@/observe/conversation"

export type PreviewStatus =
  | { kind: "invalid" }
  | { kind: "waiting" }
  | { kind: "running" }
  | { kind: "ok"; executedAt: string; series: number }
  | { kind: "error"; text: string }

export function seriesCount(
  frames: readonly Pick<WireFrame, "fields">[]
): number {
  let count = 0
  for (const frame of frames) {
    for (const field of frame.fields) {
      if (field.type === "number") count += 1
    }
  }
  return count
}

export function previewErrorText(error: unknown): string {
  if (!(error instanceof ApiError)) return "The preview could not run."
  switch (error.code) {
    case "invalid_spec": {
      const first = error.issues[0]
      return first
        ? `The draft is not valid: ${first.message}`
        : "The draft is not valid."
    }
    case "too_many_requests":
      return "Too many requests right now. The preview runs again in a moment."
    case "datasource_unavailable":
      return error.ref === undefined
        ? "The datasource is not answering."
        : `Query ${error.ref} failed: the datasource is not answering.`
    case "query_timeout":
      return error.ref === undefined
        ? "The draft took too long to run."
        : `Query ${error.ref} failed: it took too long to run.`
    default:
      return failedQueryText(error)
  }
}

export function previewStatusText(status: PreviewStatus, now: number): string {
  switch (status.kind) {
    case "invalid":
      return "The draft is not valid yet."
    case "waiting":
      return "Waiting for variables."
    case "running":
      return "Running the draft."
    case "ok":
      return `Draft ran ${lowerFirst(formatShortRelative(status.executedAt, now))}, ${status.series} series`
    case "error":
      return status.text
  }
}
