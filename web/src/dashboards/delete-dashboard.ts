import { ApiError } from "@/api/errors"
import { loadProblemText } from "@/service-accounts/queries"

export type DeleteOutcome = { kind: "gone" } | { kind: "problem"; text: string }

export function deleteDescription(versions: number): string {
  return `This deletes it for everyone, with its ${versions} ${versions === 1 ? "version" : "versions"}, comments and agent history. This cannot be undone.`
}

export function deleteOutcome(error: unknown): DeleteOutcome {
  if (error instanceof ApiError) {
    if (error.status === 404) return { kind: "gone" }
    if (error.code === "dashboard_being_created") {
      return {
        kind: "problem",
        text: "The agent is still creating this dashboard. Try again when it finishes.",
      }
    }
  }
  return { kind: "problem", text: loadProblemText(error) }
}

export function neighbourAfterDelete(
  ids: readonly string[],
  deletedId: string
): string | null {
  const index = ids.indexOf(deletedId)
  if (index === -1) return null
  return ids[index + 1] ?? ids[index - 1] ?? null
}
