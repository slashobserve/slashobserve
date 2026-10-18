import type { ApiIssue } from "./errors"

const transientCodes = new Set([
  "datasource_unavailable",
  "query_timeout",
  "internal",
])

export function isTransient(issue: ApiIssue): boolean {
  return transientCodes.has(issue.code)
}

const defaultTransientText =
  "This version could not be checked right now. Try again later."

const defaultUnknownDatasourceText =
  "A datasource this version uses is not configured anymore."

function issueText(
  issue: ApiIssue,
  transientText: string,
  unknownDatasourceText: string
): string {
  if (issue.code === "unknown_datasource") {
    return unknownDatasourceText
  }
  if (isTransient(issue)) {
    return transientText
  }
  return issue.message
}

export function issueLines(
  issues: ApiIssue[],
  transientText = defaultTransientText,
  unknownDatasourceText = defaultUnknownDatasourceText
): string[] {
  if (issues.length === 0) return ["The saved Spec is no longer valid."]
  return [
    ...new Set(
      issues.map((issue) =>
        issueText(issue, transientText, unknownDatasourceText)
      )
    ),
  ]
}
