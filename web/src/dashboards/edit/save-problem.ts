import { ApiError } from "@/api/errors"
import { isTransient, issueLines } from "@/api/issues"

const transientText = "The panel queries could not be checked right now."

export type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "invalid"; heading: string; lines: string[] }
  | { kind: "failed"; message: string }

export function saveProblemFrom(
  error: unknown,
  rejectedHeading: string
): Problem {
  if (error instanceof ApiError) {
    if (
      error.code === "version_conflict" &&
      error.latestVersion !== undefined
    ) {
      return { kind: "conflict", latest: error.latestVersion }
    }
    if (error.status === 422) {
      if (error.issues.length > 0 && error.issues.every(isTransient)) {
        return {
          kind: "failed",
          message: issueLines(error.issues, transientText).join(" "),
        }
      }
      return {
        kind: "invalid",
        heading: rejectedHeading,
        lines:
          error.issues.length > 0 ? issueLines(error.issues) : [error.message],
      }
    }
    return { kind: "failed", message: error.message }
  }
  return {
    kind: "failed",
    message: error instanceof Error ? error.message : "The save failed.",
  }
}
