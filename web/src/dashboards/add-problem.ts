import { ApiError } from "@/api/errors"
import { issueLines } from "@/api/issues"
import { exploreErrorText } from "@/logs/explore"

export type Problem = { lines: string[]; hint?: string }

const saveRefusedHint =
  "A dashboard save runs every query over the dashboard's own time range, so a query with no lines in that range is refused."

export function addProblem(error: unknown): Problem {
  if (error instanceof ApiError) {
    if (error.code === "version_conflict") {
      return {
        lines: [
          "Someone saved this dashboard a moment ago. Add again to place the panel on the latest version.",
        ],
      }
    }
    if (error.status === 409) return { lines: [error.message] }
    if (error.status === 422) {
      return {
        lines:
          error.issues.length > 0 ? issueLines(error.issues) : [error.message],
        hint: saveRefusedHint,
      }
    }
    if (error.status === 403)
      return { lines: ["You cannot edit this dashboard."] }
  }
  return { lines: [exploreErrorText(error)] }
}
