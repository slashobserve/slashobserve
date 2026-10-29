import type { ApiIssue } from "@/api/errors"
import { isTransient, issueLines } from "@/api/issues"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"

import type { JsonIssue } from "./panel-json"

const transientText = "The panel could not be checked right now."
const unknownDatasourceText =
  "A datasource this panel uses is not configured anymore."

export type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "gone" }
  | {
      kind: "rejected"
      issues: ApiIssue[]
      pointer: string
      refs: (string | null)[]
    }
  | { kind: "transient" }
  | { kind: "invalid"; issues: JsonIssue[] }
  | { kind: "clash"; version: number; issues: JsonIssue[] }
  | { kind: "failed"; message: string }

type Rejected = {
  byQuery: Map<string, ApiIssue[]>
  panel: ApiIssue[]
  dashboard: ApiIssue[]
}

function queryRefOf(
  issue: ApiIssue,
  pointer: string,
  refs: readonly (string | null)[]
): string | null | undefined {
  const queries = `${pointer}/queries/`
  if (!issue.path.startsWith(queries)) return undefined
  return refs[Number(issue.path.slice(queries.length).split("/")[0])]
}

export function splitRejected(
  issues: ApiIssue[],
  pointer: string,
  refs: readonly (string | null)[]
): Rejected {
  const own = `${pointer}/`
  const out: Rejected = { byQuery: new Map(), panel: [], dashboard: [] }
  for (const issue of issues.filter((item) => !isTransient(item))) {
    const ref = queryRefOf(issue, pointer, refs)
    if (typeof ref === "string") {
      out.byQuery.set(ref, [...(out.byQuery.get(ref) ?? []), issue])
      continue
    }
    if (issue.path === pointer || issue.path.startsWith(own)) {
      out.panel.push(issue)
    } else {
      out.dashboard.push(issue)
    }
  }
  return out
}

export function withoutQuery(
  problem: Problem | null,
  ref: string
): Problem | null {
  if (problem?.kind !== "rejected") return problem
  const issues = problem.issues.filter(
    (issue) => queryRefOf(issue, problem.pointer, problem.refs) !== ref
  )
  if (issues.length === problem.issues.length) return problem
  if (issues.length === 0) return null
  return {
    ...problem,
    issues,
    refs: problem.refs.map((item) => (item === ref ? null : item)),
  }
}

export function rejectedQueryLines(
  problem: Problem | null
): Map<string, string[]> {
  if (problem?.kind !== "rejected") return new Map()
  const { byQuery } = splitRejected(
    problem.issues,
    problem.pointer,
    problem.refs
  )
  return new Map(
    [...byQuery].map(([ref, issues]) => [
      ref,
      issueLines(issues, transientText, unknownDatasourceText),
    ])
  )
}

function onlyInRows(problem: Extract<Problem, { kind: "rejected" }>): boolean {
  const { byQuery, panel, dashboard } = splitRejected(
    problem.issues,
    problem.pointer,
    problem.refs
  )
  return (
    !problem.issues.some(isTransient) &&
    byQuery.size > 0 &&
    panel.length === 0 &&
    dashboard.length === 0
  )
}

export function showsProblem(problem: Problem | null): boolean {
  if (problem === null || problem.kind === "conflict") return false
  return problem.kind !== "rejected" || !onlyInRows(problem)
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
      problem.pointer,
      problem.refs
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
    if (
      !retryable &&
      byQuery.size === 0 &&
      panel.length === 0 &&
      dashboard.length === 0
    ) {
      return (
        <Alert variant="destructive">
          <AlertDescription>This panel was rejected.</AlertDescription>
        </Alert>
      )
    }
    if (onlyInRows(problem)) return null
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
