import { useEffect, useId, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  CircleAlertIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { toast } from "sonner"

import { ApiError, type ApiIssue } from "@/api/errors"
import { isTransient, issueLines } from "@/api/issues"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Skeleton } from "@/components/ui/skeleton"
import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "@/dashboards/queries"
import { cn } from "@/lib/utils"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import {
  findPanel,
  isDataPanel,
  validateSpecSchema,
  type Dashboard,
  type DataPanel,
  type SpecIssue,
} from "@/spec"

import { CodeEditor, type CodeEditorHandle } from "./code-editor"
import { lineAndColumn } from "./line-column"
import type { LintResult } from "./server-lint"

const maxQueryLength = 20000
const maxTitleLength = 200
const maxSummaryLength = 500
const transientText = "The query could not be checked right now."
const unknownDatasourceText =
  "A datasource this query uses is not configured anymore."

type Props = {
  dashboardId: string
  spec: Dashboard
  panel: DataPanel
  version: number
  latestVersion: number
  readOnly: boolean
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
  onOpenLatest: () => void
}

type Base = {
  version: number
  query: string
  title: string
  datasource: string
}

type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "gone" }
  | { kind: "rejected"; issues: ApiIssue[]; pointer: string }
  | { kind: "transient" }
  | { kind: "schema"; issues: SpecIssue[] }
  | { kind: "failed"; message: string }

function focusIsFree() {
  const active = document.activeElement
  return !active || active === document.body
}

function hasCoarsePointer() {
  return window.matchMedia("(pointer: coarse)").matches
}

function withDraft(
  source: Dashboard,
  panelId: string,
  base: Base,
  query: string,
  title: string
): { spec: Dashboard; pointer: string } | null {
  const next = structuredClone(source)
  const entry = findPanel(next, panelId)
  if (!entry || !isDataPanel(entry.panel)) return null
  if (query !== base.query) entry.panel.queries[0].query = query
  if (title !== base.title) entry.panel.title = title
  return { spec: next, pointer: entry.pointer }
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

function ProblemAlert({
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
    const prefix = `${problem.pointer}/`
    const own = problem.issues.filter((issue) => issue.path.startsWith(prefix))
    const others = problem.issues.filter(
      (issue) => !issue.path.startsWith(prefix)
    )
    const ownReal = own.filter((issue) => !isTransient(issue))
    const ownLines =
      ownReal.length > 0
        ? issueLines(ownReal, transientText, unknownDatasourceText)
        : []
    const otherReal = others.filter((issue) => !isTransient(issue))
    const otherLines =
      otherReal.length > 0
        ? issueLines(
            otherReal.map((issue) => ({
              ...issue,
              message: `${issue.path}: ${issue.message}`,
            })),
            transientText,
            unknownDatasourceText
          )
        : []
    const retryable = problem.issues.some(isTransient)
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-wrap items-start justify-between gap-3 text-pretty [&_p:not(:last-child)]:mb-0">
          <div className="flex min-w-[min(100%,16rem)] flex-1 flex-col gap-2">
            {ownLines.length > 0 ? (
              <div>
                <p className="font-medium">This query was rejected:</p>
                <IssueList lines={ownLines} />
              </div>
            ) : null}
            {otherLines.length > 0 ? (
              <div>
                <p className="font-medium">Other problems in this dashboard:</p>
                <IssueList lines={otherLines} />
              </div>
            ) : null}
            {retryable ? <p>{transientText}</p> : null}
            {problem.issues.length === 0 ? (
              <p>This query was rejected.</p>
            ) : null}
          </div>
          {retryable ? <RetryButton onRetry={onRetry} /> : null}
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "schema") {
    return (
      <Alert variant="destructive">
        <AlertDescription className="[&_p:not(:last-child)]:mb-0">
          <p className="font-medium">
            The panel {panelId} would not be a valid Spec:
          </p>
          <IssueList
            lines={problem.issues.map(
              (issue) => `${issue.path || "/"}: ${issue.message}`
            )}
          />
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

function Diagnostics({
  lint,
  query,
  changed,
  onPick,
}: {
  lint: LintResult | null
  query: string
  changed: boolean
  onPick: (from: number) => void
}) {
  const current = lint?.query === query ? lint : null
  let content = null
  if (current?.status === "unavailable") {
    content = <li className="text-muted-foreground">{current.message}</li>
  } else if (current?.status === "ok" && current.items.length > 0) {
    content = current.items.map((item, index) => {
      const { line, column } = lineAndColumn(current.query, item.from)
      const isError = item.severity === "error"
      const Icon = isError ? CircleAlertIcon : TriangleAlertIcon
      return (
        <li key={index}>
          <Button
            variant="ghost"
            size="sm"
            className="h-auto w-full items-start justify-start py-1 text-left whitespace-normal"
            onClick={() => onPick(item.from)}
          >
            <Icon
              aria-hidden="true"
              className={cn("mt-0.5", isError && "text-destructive")}
            />
            <span>
              {isError ? "Error" : "Warning"}: {item.message} (line {line},
              column {column})
            </span>
          </Button>
        </li>
      )
    })
  } else if (current?.status === "ok" && changed) {
    content = <li className="text-muted-foreground">No problems found</li>
  }
  return (
    <ul aria-live="polite" className="flex flex-col gap-0.5 text-sm">
      {content}
    </ul>
  )
}

export default function QueryTab({
  dashboardId,
  spec,
  panel,
  version,
  latestVersion,
  readOnly,
  onDirtyChange,
  onSaved,
  onOpenLatest,
}: Props) {
  const titleId = useId()
  const summaryId = useId()
  const queryClient = useQueryClient()
  const save = useAddVersion(dashboardId)
  const editor = useRef<CodeEditorHandle>(null)
  const notice = useRef<HTMLDivElement>(null)
  const lastTarget = useRef<{ source: Dashboard; parent: number } | null>(null)

  const [base, setBase] = useState<Base>(() => ({
    version,
    query: panel.queries[0].query,
    title: panel.title,
    datasource: panel.queries[0].datasource ?? panel.datasource,
  }))
  const datasources = useDatasources()
  const [draft, setDraft] = useState(panel.queries[0].query)
  const [title, setTitle] = useState(panel.title)
  const [summaryDraft, setSummaryDraft] = useState<string | null>(null)
  const [lint, setLint] = useState<LintResult | null>(null)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [applying, setApplying] = useState(false)

  const dirty = draft !== base.query || title.trim() !== base.title

  if (!dirty && version !== base.version) {
    setBase({
      version,
      query: panel.queries[0].query,
      title: panel.title,
      datasource: panel.queries[0].datasource ?? panel.datasource,
    })
    setDraft(panel.queries[0].query)
    setTitle(panel.title)
  }

  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange(false), [onDirtyChange])

  const summary =
    summaryDraft ?? `Edit query of "${title.trim() || base.title}"`
  const saving = save.isPending || save.isSuccess
  const newer = saving
    ? 0
    : Math.max(
        version > base.version ? version : 0,
        latestVersion > base.version ? latestVersion : 0,
        problem?.kind === "conflict" ? problem.latest : 0
      )
  const conflicted = problem?.kind === "conflict"
  const hadNewer = useRef(false)
  useEffect(() => {
    if (conflicted) notice.current?.focus()
  }, [conflicted])
  useEffect(() => {
    if (newer > 0 && !hadNewer.current && !conflicted && focusIsFree()) {
      notice.current?.focus()
    }
    hadNewer.current = newer > 0
  }, [newer, conflicted])
  const tooLong = [...draft].length > maxQueryLength
  const canSave =
    dirty &&
    !saving &&
    !applying &&
    draft.trim() !== "" &&
    !tooLong &&
    title.trim() !== ""

  function submit(source: Dashboard, parent: number) {
    lastTarget.current = { source, parent }
    const built = withDraft(source, panel.id, base, draft, title.trim())
    if (!built) {
      setProblem({ kind: "gone" })
      return
    }
    const checked = validateSpecSchema(built.spec)
    if (!checked.valid) {
      setProblem({ kind: "schema", issues: checked.issues })
      return
    }
    setProblem(null)
    save.mutate(
      {
        spec: checked.spec,
        summary: summary.trim() || `Edit query of "${title.trim()}"`,
        parent_version: parent,
      },
      {
        onSuccess: (saved) => {
          toast.success(`Saved version ${saved.version.version}`)
          onSaved()
        },
        onError: (error) => {
          if (error instanceof ApiError) {
            if (error.status === 409 && error.latestVersion !== undefined) {
              setProblem({ kind: "conflict", latest: error.latestVersion })
              return
            }
            if (error.status === 422) {
              if (error.issues.length > 0 && error.issues.every(isTransient)) {
                setProblem({ kind: "transient" })
                return
              }
              setProblem({
                kind: "rejected",
                issues: error.issues,
                pointer: built.pointer,
              })
              return
            }
          }
          setProblem({
            kind: "failed",
            message:
              error instanceof Error ? error.message : "The save failed.",
          })
        },
      }
    )
  }

  async function applyToLatest() {
    setApplying(true)
    setProblem(null)
    try {
      const latest = await queryClient.fetchQuery({
        ...latestDashboardQuery(queryClient, dashboardId),
        staleTime: 0,
      })
      submit(asDashboardSpec(latest.version.spec), latest.version.version)
    } catch (error) {
      setProblem({
        kind: "failed",
        message:
          error instanceof Error
            ? error.message
            : "Could not load the latest version.",
      })
    } finally {
      setApplying(false)
    }
  }

  function reset() {
    setDraft(base.query)
    setTitle(base.title)
    setProblem(null)
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <Label htmlFor={titleId}>Title</Label>
        <Input
          id={titleId}
          value={title}
          maxLength={maxTitleLength}
          readOnly={readOnly}
          className={cn(
            readOnly &&
              "border-transparent bg-muted/50 text-muted-foreground dark:bg-muted/30"
          )}
          onChange={(event) => setTitle(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        {datasources.isPending ? (
          <Skeleton className="h-40 w-full animate-none motion-safe:animate-pulse" />
        ) : (
          <CodeEditor
            value={draft}
            onChange={setDraft}
            datasource={{
              uid: base.datasource,
              type: datasourceTypeOf(datasources.data, base.datasource),
            }}
            readOnly={readOnly}
            ariaLabel={`Query for ${base.title}`}
            onLint={setLint}
            editorRef={editor}
            autoFocus={!readOnly && !hasCoarsePointer()}
          />
        )}
        {tooLong ? (
          <p className="text-sm text-destructive">
            The query is longer than {maxQueryLength} characters.
          </p>
        ) : null}
        <Diagnostics
          lint={lint}
          query={draft}
          changed={dirty}
          onPick={(from) => editor.current?.focusAt(from)}
        />
      </div>
      {readOnly ? (
        <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-4">
          <p className="text-sm text-muted-foreground">
            Open the latest version to edit this query.
          </p>
          <Button variant="outline" onClick={onOpenLatest}>
            Open latest version
          </Button>
        </div>
      ) : (
        <div className="flex flex-col gap-4 border-t pt-4">
          {newer > 0 ? (
            <Alert
              ref={notice}
              tabIndex={-1}
              className="outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
            >
              <AlertTitle>
                Version {newer} was saved while you were editing.
              </AlertTitle>
              <AlertDescription>
                Your query stays here until you apply it to that version.
              </AlertDescription>
            </Alert>
          ) : null}
          {problem ? (
            <ProblemAlert
              problem={problem}
              panelId={panel.id}
              onRetry={() => {
                if (newer > 0) {
                  void applyToLatest()
                  return
                }
                const target = lastTarget.current
                if (target) submit(target.source, target.parent)
              }}
            />
          ) : null}
          <div className="flex flex-col gap-2">
            <Label htmlFor={summaryId}>Summary</Label>
            <Input
              id={summaryId}
              value={summary}
              maxLength={maxSummaryLength}
              onChange={(event) => setSummaryDraft(event.target.value)}
            />
          </div>
          {saving || applying ? (
            <p
              role="status"
              className="flex items-center gap-2 text-sm text-muted-foreground"
            >
              <LoaderCircleIcon
                aria-hidden="true"
                className="size-4 motion-safe:animate-spin"
              />
              Saving and running every panel query.
            </p>
          ) : null}
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              variant="ghost"
              disabled={!dirty || saving || applying}
              onClick={reset}
            >
              Reset
            </Button>
            <Button
              disabled={!canSave}
              onClick={() =>
                newer > 0 ? void applyToLatest() : submit(spec, base.version)
              }
            >
              {newer > 0
                ? `Apply my query to version ${newer}`
                : "Save as new version"}
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
