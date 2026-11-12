import { ApiError } from "@/api/errors"
import { loadProblemText } from "@/service-accounts/queries"

const staleCodes = new Set([
  "proposal_changed",
  "version_conflict",
  "invalid_transition",
])

export function mergeProblem(error: unknown): string {
  if (error instanceof ApiError) {
    switch (error.code) {
      case "proposal_changed":
        return "This proposal has a newer revision. Review it and merge again."
      case "version_conflict":
        return "The dashboard changed while merging. Review the conflicts and merge again."
      case "invalid_transition":
        return "This proposal is already closed."
    }
  }
  return loadProblemText(error)
}

export function mergeLeftProposalStale(error: unknown): boolean {
  return error instanceof ApiError && staleCodes.has(error.code)
}
