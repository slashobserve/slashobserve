import {
  useEffect,
  useId,
  useImperativeHandle,
  useMemo,
  useReducer,
  useRef,
  useState,
  type Ref,
} from "react"
import { useQueryClient } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"
import { toast } from "sonner"

import type { Selections } from "@/api/keys"
import { ApiError } from "@/api/errors"
import { isTransient } from "@/api/issues"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  asDashboardSpec,
  latestDashboardQuery,
  useAddVersion,
} from "@/dashboards/queries"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import type { CodeEditorHandle } from "@/query-editor/code-editor"
import { findPanel, isDataPanel, type Dashboard } from "@/spec"

import { AskAgent } from "./ask-agent"
import {
  changeSummary,
  defaultSaveSummary,
  panelChanges,
  rebasePanel,
} from "./changes"
import { DatasourceSelect } from "./datasource-select"
import {
  draftReducer,
  isEditablePanel,
  startDraft,
  type EditablePanel,
} from "./draft"
import { EditorFooter } from "./editor-footer"
import { JsonView } from "./json-view"
import { MetricsBrowser } from "./metrics-browser"
import { parsePanelJson, withPanel } from "./panel-json"
import { PreviewSection } from "./preview-section"
import { maxQueryLength } from "./query-row"
import { QueryRows, maxContentLength } from "./query-rows"
import { SaveProblem, rejectedQueryLines, type Problem } from "./save-problem"
import { SettingsColumn } from "./settings-column"

export type EditorView = "editor" | "json"

export type ViewGuard = { request: (view: EditorView) => void }

type Props = {
  dashboardId: string
  spec: Dashboard
  panel: EditablePanel
  version: number
  latestVersion: number
  readOnly: boolean
  view: EditorView
  onViewChange: (view: EditorView) => void
  viewGuardRef: Ref<ViewGuard>
  timeRange: { from?: string; to?: string }
  selections: Selections | null
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
  onOpenLatest: () => void
  onDiscard: () => void
}

const savingText = "Saving and running every panel query."
const readOnlyAskReason = "Open the latest version to ask the agent."

function focusIsFree() {
  const active = document.activeElement
  return !active || active === document.body
}

function normalise(panel: EditablePanel): EditablePanel {
  const next = structuredClone(panel)
  if (next.title !== undefined) next.title = next.title.trim()
  if (isDataPanel(next)) {
    for (const query of next.queries) {
      const legend = query.legend?.trim()
      if (legend) query.legend = legend
      else delete query.legend
    }
  }
  return next
}

function leaveJson(leave: () => void) {
  leave()
}

export default function PanelEditor({
  dashboardId,
  spec,
  panel,
  version,
  latestVersion,
  readOnly,
  view,
  onViewChange,
  viewGuardRef,
  timeRange,
  selections,
  onDirtyChange,
  onSaved,
  onOpenLatest,
  onDiscard,
}: Props) {
  const datasourceId = useId()
  const queryClient = useQueryClient()
  const save = useAddVersion(dashboardId)
  const datasources = useDatasources()
  const [draft, dispatch] = useReducer(draftReducer, undefined, () =>
    startDraft(version, panel)
  )
  const changes = useMemo(
    () => panelChanges(draft.base, draft.panel),
    [draft.base, draft.panel]
  )
  const dirty = changes.length > 0
  const [jsonValid, setJsonValid] = useState(true)
  const [metricsOpen, setMetricsOpen] = useState(false)
  const [activeQuery, setActiveQuery] = useState(0)
  const [note, setNote] = useState("")
  const [noteOpen, setNoteOpen] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [applying, setApplying] = useState(false)
  const editorRefs = useRef<(CodeEditorHandle | null)[]>([])
  const noticeRef = useRef<HTMLDivElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)
  const lastTarget = useRef<{
    source: Dashboard
    parent: number
    panel: EditablePanel
  } | null>(null)

  if (!dirty && version !== draft.baseVersion) {
    dispatch({ type: "rebase", version, panel })
  }

  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange(false), [onDirtyChange])

  useImperativeHandle(
    viewGuardRef,
    () => ({
      request: (next) => {
        if (next === view) return
        if (view === "json") leaveJson(() => onViewChange(next))
        else onViewChange(next)
      },
    }),
    [view, onViewChange]
  )

  const saving = save.isPending || save.isSuccess
  const newer = saving
    ? 0
    : Math.max(
        version > draft.baseVersion ? version : 0,
        latestVersion > draft.baseVersion ? latestVersion : 0,
        problem?.kind === "conflict" ? problem.latest : 0
      )
  const conflicted = problem?.kind === "conflict"
  const hadNewer = useRef(false)
  useEffect(() => {
    if (conflicted) noticeRef.current?.focus()
  }, [conflicted])
  useEffect(() => {
    if (newer > 0 && !hadNewer.current && !conflicted && focusIsFree()) {
      noticeRef.current?.focus()
    }
    hadNewer.current = newer > 0
  }, [newer, conflicted])

  const issuesByQuery = useMemo(() => rejectedQueryLines(problem), [problem])
  useEffect(() => {
    if (Object.keys(issuesByQuery).length === 0) return
    bodyRef.current
      ?.querySelector('[role="alert"]')
      ?.scrollIntoView({ block: "nearest" })
  }, [issuesByQuery])

  const current = draft.panel
  const jsonBlocked = view === "json" && !jsonValid
  const titleOk = current.title === undefined || current.title.trim() !== ""
  const queriesOk = isDataPanel(current)
    ? current.queries.every(
        (query) =>
          query.query.trim() !== "" && [...query.query].length <= maxQueryLength
      )
    : [...current.content].length <= maxContentLength
  const canSave =
    dirty &&
    !saving &&
    !applying &&
    !jsonBlocked &&
    !readOnly &&
    queriesOk &&
    titleOk

  function submit(
    source: Dashboard,
    parent: number,
    panelToSave: EditablePanel
  ) {
    lastTarget.current = { source, parent, panel: panelToSave }
    const entry = findPanel(source, panelToSave.id)
    if (!entry) {
      setProblem({ kind: "gone" })
      return
    }
    const normalised = normalise(panelToSave)
    const parsed = parsePanelJson(
      JSON.stringify(normalised),
      source,
      normalised.id
    )
    if (!parsed.ok) {
      setProblem(
        parent === draft.baseVersion
          ? { kind: "invalid", issues: parsed.issues }
          : { kind: "clash", version: parent, issues: parsed.issues }
      )
      return
    }
    const placed = withPanel(source, parsed.panel.id, parsed.panel)
    if (!placed) {
      setProblem({ kind: "gone" })
      return
    }
    setProblem(null)
    save.mutate(
      {
        spec: placed,
        summary:
          note.trim() ||
          defaultSaveSummary(changes, normalised.title || normalised.id),
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
                pointer: entry.pointer,
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
      const latestSpec = asDashboardSpec(latest.version.spec)
      const entry = findPanel(latestSpec, draft.panel.id)
      if (!entry) {
        setProblem({ kind: "gone" })
        return
      }
      if (!isEditablePanel(entry.panel)) {
        setProblem({
          kind: "failed",
          message: "This panel can no longer be edited in the latest version.",
        })
        return
      }
      submit(
        latestSpec,
        latest.version.version,
        rebasePanel(draft.base, draft.panel, entry.panel)
      )
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

  function retry() {
    if (newer > 0) {
      void applyToLatest()
      return
    }
    const target = lastTarget.current
    if (target) submit(target.source, target.parent, target.panel)
  }

  const data = isDataPanel(current) ? current : null
  const active = data?.queries[Math.min(activeQuery, data.queries.length - 1)]
  const activeUid = active?.datasource ?? data?.datasource ?? ""

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        ref={bodyRef}
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-auto px-4 py-3.5"
      >
        <PreviewSection
          dashboardId={dashboardId}
          spec={spec}
          version={version}
          base={draft.base}
          draft={draft.panel}
          timeRange={timeRange}
          selections={selections}
        />
        {view === "editor" ? (
          <>
            <AskAgent
              dashboardId={dashboardId}
              version={draft.baseVersion}
              draft={draft.panel}
              dispatch={dispatch}
              blockedReason={readOnly ? readOnlyAskReason : null}
            />
            <div className="grid grid-cols-[minmax(0,1fr)_220px] items-start gap-5 narrow:grid-cols-1">
              <div className="flex min-w-0 flex-col gap-3">
                {data ? (
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Label htmlFor={datasourceId} className="text-[13px]">
                        Datasource
                      </Label>
                      <DatasourceSelect
                        id={datasourceId}
                        value={data.datasource}
                        disabled={readOnly}
                        onChange={(uid) =>
                          dispatch({ type: "setPanelDatasource", uid })
                        }
                      />
                    </div>
                    <Button
                      variant="outline"
                      size="sm"
                      className="text-xs"
                      aria-expanded={metricsOpen}
                      onClick={() => setMetricsOpen((open) => !open)}
                    >
                      <SearchIcon aria-hidden="true" className="size-[13px]" />
                      Metrics and labels
                    </Button>
                  </div>
                ) : null}
                <QueryRows
                  draft={draft.panel}
                  base={draft.base}
                  dispatch={dispatch}
                  readOnly={readOnly}
                  issuesByQuery={issuesByQuery}
                  onActiveChange={setActiveQuery}
                  editorRefs={editorRefs}
                  beforeAdd={
                    metricsOpen && data ? (
                      <MetricsBrowser
                        datasourceUid={activeUid}
                        datasourceType={datasourceTypeOf(
                          datasources.data,
                          activeUid
                        )}
                        onInsert={(text) =>
                          editorRefs.current[activeQuery]?.insertAtCursor(text)
                        }
                      />
                    ) : null
                  }
                />
              </div>
              <SettingsColumn
                draft={draft.panel}
                dispatch={dispatch}
                readOnly={readOnly}
              />
            </div>
          </>
        ) : (
          <JsonView
            spec={spec}
            draft={draft.panel}
            dispatch={dispatch}
            readOnly={readOnly}
            onValidityChange={setJsonValid}
            onLeaveRequest={leaveJson}
          />
        )}
      </div>
      {readOnly || (newer === 0 && problem === null) ? null : (
        <div className="flex max-h-[40%] flex-col gap-2 overflow-auto px-4 pb-3">
          {newer > 0 ? (
            <Alert
              ref={noticeRef}
              tabIndex={-1}
              className="outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
            >
              <AlertTitle>
                Version {newer} was saved while you were editing.
              </AlertTitle>
              <AlertDescription>
                Your changes stay here until you apply them to that version.
              </AlertDescription>
            </Alert>
          ) : null}
          {problem ? (
            <SaveProblem
              problem={problem}
              panelId={current.id}
              onRetry={retry}
            />
          ) : null}
        </div>
      )}
      <EditorFooter
        summary={jsonBlocked ? "Fix the JSON to save." : changeSummary(changes)}
        noteOpen={noteOpen}
        note={note}
        onNoteOpen={() => setNoteOpen(true)}
        onNoteChange={setNote}
        saving={saving || applying}
        canSave={canSave}
        saveLabel={
          newer > 0
            ? `Apply my changes to version ${newer}`
            : `Save as version ${latestVersion + 1}`
        }
        onSave={() =>
          newer > 0
            ? void applyToLatest()
            : submit(spec, draft.baseVersion, draft.panel)
        }
        onDiscard={onDiscard}
        readOnly={readOnly}
        onOpenLatest={onOpenLatest}
        status={saving || applying ? savingText : null}
      />
    </div>
  )
}
