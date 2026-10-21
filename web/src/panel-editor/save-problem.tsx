import type { ApiIssue } from "@/api/errors"
import { isTransient, issueLines } from "@/api/issues"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

import type { JsonIssue } from "./panel-json"

export const transientText = "The panel could not be checked right now."
const unknownDatasourceText =
  "A datasource this panel uses is not configured anymore."

export type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "gone" }
  | { kind: "rejected"; issues: ApiIssue[]; pointer: string }
  | { kind: "transient" }
  | { kind: "invalid"; issues: JsonIssue[] }
  | { kind: "clash"; version: number; issues: JsonIssue[] }
  | { kind: "failed"; message: string }

type Rejected = {
  byQuery: Record<number, ApiIssue[]>
  panel: ApiIssue[]
  dashboard: ApiIssue[]
}

function splitRejected(issues: ApiIssue[], pointer: string): Rejected {
  const queries = `${pointer}/queries/`
  const own = `${pointer}/`
  const out: Rejected = { byQuery: {}, panel: [], dashboard: [] }
  for (const issue of issues.filter((item) => !isTransient(item))) {
    if (issue.path.startsWith(queries)) {
      const index = Number(issue.path.slice(queries.length).split("/")[0])
      if (Number.isInteger(index)) {
        out.byQuery[index] = [...(out.byQuery[index] ?? []), issue]
        continue
      }
    }
    if (issue.path === pointer || issue.path.startsWith(own)) {
      out.panel.push(issue)
    } else {
      out.dashboard.push(issue)
    }
  }
  return out
}

export function rejectedQueryLines(
  problem: Problem | null
): Record<number, string[]> {
  if (problem?.kind !== "rejected") return {}
  const { byQuery } = splitRejected(problem.issues, problem.pointer)
  return Object.fromEntries(
    Object.entries(byQuery).map(([index, issues]) => [
      index,
      issueLines(issues, transientText, unknownDatasourceText),
    ])
  )
}

function IssueList({ lines }: { lines: string[] }) {
  return (
    <ul className="list-disc pl-4">
      {lines.map((line, index) => (
        <li key={index}>{line}</li>
      ))}
    </ul>
  )
}

function withPath(issues: { path: string; message: string }[]): string[] {
  return issues.map((issue) => `${issue.path || "/"}: ${issue.message}`)
}

function RetryButton({ onRetry }: { onRetry: () => void }) {
  return (
    <Button
      variant="outline"
      size="sm"
      className="shrink-0 text-foreground"
      onClick={onRetry}
    >
      Try again
    </Button>
  )
}

function Lists({ intro, issues }: { intro: string; issues: JsonIssue[] }) {
  const draft = issues.filter((issue) => issue.scope === "panel")
  const version = issues.filter((issue) => issue.scope === "dashboard")
  return (
    <>
      {draft.length > 0 ? (
        <div>
          <p className="font-medium">{intro}</p>
          <IssueList lines={withPath(draft)} />
        </div>
      ) : null}
      {version.length > 0 ? (
        <div>
          <p className="font-medium">Other problems in this dashboard:</p>
          <IssueList lines={withPath(version)} />
        </div>
      ) : null}
    </>
  )
}

export function SaveProblem({
  problem,
  panelId,
  onRetry,
}: {
  problem: Problem
  panelId: string
  onRetry: () => void
}) {
  if (problem.kind === "transient") {
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-wrap items-center justify-between gap-3 text-pretty">
          <span className="min-w-[min(100%,16rem)] flex-1">
            {transientText}
          </span>
          <RetryButton onRetry={onRetry} />
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "rejected") {
    const { byQuery, panel, dashboard } = splitRejected(
      problem.issues,
      problem.pointer
    )
    const retryable = problem.issues.some(isTransient)
    const panelLines = issueLines(panel, transientText, unknownDatasourceText)
    const dashboardLines = issueLines(
      dashboard.map((issue) => ({
        ...issue,
        message: `${issue.path}: ${issue.message}`,
      })),
      transientText,
      unknownDatasourceText
    )
    const inRows = Object.keys(byQuery).length > 0
    if (!retryable && !inRows && panel.length === 0 && dashboard.length === 0) {
      return (
        <Alert variant="destructive">
          <AlertDescription>This panel was rejected.</AlertDescription>
        </Alert>
      )
    }
    if (!retryable && panel.length === 0 && dashboard.length === 0) return null
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-wrap items-start justify-between gap-3 text-pretty [&_p:not(:last-child)]:mb-0">
          <div className="flex min-w-[min(100%,16rem)] flex-1 flex-col gap-2">
            {panel.length > 0 ? (
              <div>
                <p className="font-medium">This panel was rejected:</p>
                <IssueList lines={panelLines} />
              </div>
            ) : null}
            {dashboard.length > 0 ? (
              <div>
                <p className="font-medium">Other problems in this dashboard:</p>
                <IssueList lines={dashboardLines} />
              </div>
            ) : null}
            {retryable ? <p>{transientText}</p> : null}
          </div>
          {retryable ? <RetryButton onRetry={onRetry} /> : null}
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "invalid") {
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-col gap-2 [&_p:not(:last-child)]:mb-0">
          <Lists
            intro={`The panel ${panelId} would not be a valid Spec:`}
            issues={problem.issues}
          />
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "clash") {
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-col gap-2 [&_p:not(:last-child)]:mb-0">
          <p className="font-medium">
            Your changes do not fit version {problem.version}.
          </p>
          <Lists intro="What does not fit:" issues={problem.issues} />
          <p>
            Change the panel to fit that version, or discard your changes and
            start from it.
          </p>
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "gone") {
    return (
      <Alert variant="destructive">
        <AlertDescription>
          This panel no longer exists in the latest version.
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "failed") {
    return (
      <Alert variant="destructive">
        <AlertDescription>{problem.message}</AlertDescription>
      </Alert>
    )
  }
  return null
}
