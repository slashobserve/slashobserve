import { useEffect, useId, useRef, useState } from "react"
import { useQueryClient } from "@tanstack/react-query"
import {
  CircleAlertIcon,
  LoaderCircleIcon,
  TriangleAlertIcon,
} from "lucide-react"
import { toast } from "sonner"

import { ApiError, type ApiIssue } from "@/api/errors"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "@/dashboards/queries"
import {
  validateSpecSchema,
  type Dashboard,
  type Panel,
  type SpecIssue,
} from "@/spec"

import { CodeEditor, type CodeEditorHandle } from "./code-editor"
import type { LintResult } from "./server-lint"

const maxQueryLength = 20000
const maxTitleLength = 200
const maxSummaryLength = 500

type Props = {
  dashboardId: string
  spec: Dashboard
  panel: Panel
  version: number
  readOnly: boolean
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
  onOpenLatest: () => void
}

type Base = {
  version: number
  query: string
  title: string
  datasource: Panel["datasource"]
}

type Problem =
  | { kind: "conflict"; latest: number }
  | { kind: "gone" }
  | { kind: "rejected"; issues: ApiIssue[]; index: number }
  | { kind: "schema"; issues: SpecIssue[] }
  | { kind: "failed"; message: string }

function lineAndColumn(text: string, offset: number) {
  const before = text.slice(0, Math.min(Math.max(offset, 0), text.length))
  const lines = before.split("\n")
  return { line: lines.length, column: lines[lines.length - 1].length + 1 }
}

function withDraft(
  source: Dashboard,
  panelId: string,
  query: string,
  title: string
): { spec: Dashboard; index: number } | null {
  const next = structuredClone(source)
  const index = next.panels.findIndex((item) => item.id === panelId)
  if (index < 0) return null
  next.panels[index].query = query
  next.panels[index].title = title
  return { spec: next, index }
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

function ProblemAlert({
  problem,
  panelId,
}: {
  problem: Problem
  panelId: string
}) {
  if (problem.kind === "rejected") {
    const prefix = `/panels/${problem.index}/`
    const own = problem.issues.filter((issue) => issue.path.startsWith(prefix))
    const others = problem.issues.filter(
      (issue) => !issue.path.startsWith(prefix)
    )
    return (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-col gap-2">
          {own.length > 0 ? (
            <div>
              <p className="font-medium">This query was rejected:</p>
              <IssueList lines={own.map((issue) => issue.message)} />
            </div>
          ) : null}
          {others.length > 0 ? (
            <div>
              <p className="font-medium">Other problems in this dashboard:</p>
              <IssueList
                lines={others.map((issue) => `${issue.path}: ${issue.message}`)}
              />
            </div>
          ) : null}
          {problem.issues.length === 0 ? <p>This query was rejected.</p> : null}
        </AlertDescription>
      </Alert>
    )
  }
  if (problem.kind === "schema") {
    return (
      <Alert variant="destructive">
        <AlertDescription>
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
  changed,
  onPick,
}: {
  lint: LintResult | null
  changed: boolean
  onPick: (from: number) => void
}) {
  let content = null
  if (lint?.status === "unavailable") {
    content = (
      <li className="text-muted-foreground">Lint is unavailable right now.</li>
    )
  } else if (lint?.status === "ok" && lint.diagnostics.length > 0) {
    content = lint.diagnostics.map((item, index) => {
      const { line, column } = lineAndColumn(lint.query, item.from)
      const isError = item.severity === "error"
      const Icon = isError ? CircleAlertIcon : TriangleAlertIcon
      return (
        <li key={index}>
          <Button
            variant="ghost"
            size="sm"
            className="h-auto w-full justify-start py-1 text-left whitespace-normal"
            onClick={() => onPick(item.from)}
          >
            <Icon
              aria-hidden="true"
              className={isError ? "text-destructive" : undefined}
            />
            <span>
              {isError ? "Error" : "Warning"}: {item.message} (line {line},
              column {column})
            </span>
          </Button>
        </li>
      )
    })
  } else if (lint?.status === "ok" && changed) {
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

  const [base, setBase] = useState<Base>(() => ({
    version,
    query: panel.query,
    title: panel.title,
    datasource: panel.datasource,
  }))
  const [draft, setDraft] = useState(panel.query)
  const [title, setTitle] = useState(panel.title)
  const [summaryDraft, setSummaryDraft] = useState<string | null>(null)
  const [lint, setLint] = useState<LintResult | null>(null)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [applying, setApplying] = useState(false)

  const dirty = draft !== base.query || title.trim() !== base.title

  if (!dirty && version !== base.version) {
    setBase({
      version,
      query: panel.query,
      title: panel.title,
      datasource: panel.datasource,
    })
    setDraft(panel.query)
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
        problem?.kind === "conflict" ? problem.latest : 0
      )
  const tooLong = [...draft].length > maxQueryLength
  const canSave =
    dirty &&
    !saving &&
    !applying &&
    draft.trim() !== "" &&
    !tooLong &&
    title.trim() !== ""

  function submit(source: Dashboard, parent: number) {
    const built = withDraft(source, panel.id, draft, title.trim())
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
              setProblem({
                kind: "rejected",
                issues: error.issues,
                index: built.index,
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
          onChange={(event) => setTitle(event.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        <CodeEditor
          value={draft}
          onChange={setDraft}
          datasource={base.datasource}
          readOnly={readOnly}
          ariaLabel={`Query for ${base.title}`}
          onLint={setLint}
          editorRef={editor}
          autoFocus={!readOnly}
        />
        {tooLong ? (
          <p className="text-sm text-destructive">
            The query is longer than {maxQueryLength} characters.
          </p>
        ) : null}
        <Diagnostics
          lint={lint}
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
            <Alert>
              <AlertTitle>
                Version {newer} was saved while you were editing.
              </AlertTitle>
              <AlertDescription>
                Your query stays here until you apply it to that version.
              </AlertDescription>
            </Alert>
          ) : null}
          {problem ? (
            <ProblemAlert problem={problem} panelId={panel.id} />
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
