import { ApiError } from "@/api/errors"
import { sentence } from "@/observe/queries"

const fallback = "Could not publish"

function serverSentence(error: ApiError): string {
  return error.message.trim() === "" ? fallback : sentence(error.message)
}

export function publishProblem(error: unknown): string {
  if (!(error instanceof ApiError)) return fallback
  switch (error.code) {
    case "rca_limit":
      return "This observation has 50 revisions, the most it can have."
    case "observation_changed":
      return "The page changed while it was being published. Publish again."
    case "panel_failed": {
      const first = error.issues[0]?.message
      return first === undefined
        ? serverSentence(error)
        : `A panel could not be frozen: ${first}`
    }
    case "snapshot_too_large":
      return "A panel returned too much data to freeze. Narrow its time range and publish again."
    default:
      return serverSentence(error)
  }
}
