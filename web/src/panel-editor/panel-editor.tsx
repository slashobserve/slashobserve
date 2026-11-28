import {
  useCallback,
  useEffect,
  useId,
  useImperativeHandle,
  useMemo,
  useReducer,
  useRef,
  useState,
  type Ref,
} from "react"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import { SearchIcon } from "lucide-react"

import type { Selections } from "@/api/keys"
import { ApiError } from "@/api/errors"
import { isTransient } from "@/api/issues"
import { ScrollPane } from "@/components/scroll-pane"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import {
  asDashboardSpec,
  latestDashboardQuery,
  type PanelPatch,
} from "@/dashboards/queries"
import { MergeConflicts } from "@/dashboards/edit/merge-conflicts"
import { datasourceTypeOf, useDatasources } from "@/datasources/queries"
import { panelTitles } from "@/history/diff-groups"
import { useSpecValidator } from "@/lib/use-spec-validator"
import type { CodeEditorHandle } from "@/query-editor/code-editor"
import {
  datasourceRefType,
  findPanel,
  isDataPanel,
  isPanelQuery,
  panelTitle,
  queryRef,
  resolveDatasourceRef,
  reusesPanel,
  type Dashboard,
  type DatasourceVariable,
} from "@/spec"

import { AskAgent } from "./ask-agent"
import {
  changeSummary,
  defaultSaveSummary,
  normalisePanel,
  overlappingChanges,
  panelChanges,
  rebasePanel,
} from "./changes"
import { DatasourceSelect } from "./datasource-select"
import {
  activeQueryIndex,
  draftReducer,
  isEditablePanel,
  startDraft,
  type EditablePanel,
} from "./draft"
import { restoreBackup, useDraftBackup } from "./draft-backup"
import { useDraftTitles } from "./draft-title"
import { EditorFooter } from "./editor-footer"
import { JsonView, type LeaveGuard } from "./json-view"
import { MetricsBrowser } from "./metrics-browser"
import { NewerVersionNotice, newerVersionAuthor } from "./newer-version-notice"
import { withNewPanel } from "./new-panel"
import { parsePanelJson } from "./panel-json"
import { PreviewSection, type PreviewTarget } from "./preview-section"
import { saveBlockedText } from "./save-blocked"
import { maxQueryLength } from "./query-row"
import type { ResolveDatasource } from "./query-row"
import { QueryRows, maxContentLength } from "./query-rows"
import {
  onlyEmptyResult,
  onlyGridOverlap,
  SaveProblem,
  rejectedQueryLines,
  showsProblem,
  querySources,
  transientLines,
  withoutQuery,
  type Problem,
} from "./save-problem"
import { SettingsColumn } from "./settings-column"

export type EditorView = "editor" | "json"

export type ViewGuard = {
  request: (view: EditorView) => void
  guardLeave: (leave: () => void, stay?: () => void) => void
}

export type PanelsSave = {
  patch: PanelPatch
  summary: string
  parent_version: number
  propose: boolean
  acceptEmpty: boolean
}

export type DashboardTarget = {
  kind: "dashboard"
  dashboardId: string
  latestVersion: number
  adding: boolean
  propose: boolean
  deleted: boolean
  save: (input: PanelsSave) => Promise<unknown>
}

export type BlockTarget = {
  kind: "block"
  observationId: string
  blockId: string | null
  saveLabel: string
  outcome: string | null
  save: (panel: EditablePanel) => Promise<unknown>
}

export type EditorTarget = DashboardTarget | BlockTarget

const blockIssuePointer = "/panel"

type Props = {
  target: EditorTarget
  spec: Dashboard
  panel: EditablePanel
  version: number
  readOnly: boolean
  view: EditorView
  focusFirstQuery: boolean
  onViewChange: (view: EditorView) => void
  viewGuardRef: Ref<ViewGuard>
  timeRange: { from?: string; to?: string }
  selections: Selections | null
  onDirtyChange: (dirty: boolean) => void
  onSaved: () => void
  onOpenLatest: () => void
  onDiscard: () => void
}

const savingText = "Saving and running this panel's queries."
const readOnlyAskReason = "Open the latest version to ask the agent."

function focusIsFree() {
  const active = document.activeElement
  return !active || active === document.body
}

const leaveNow: LeaveGuard = (leave) => leave()

function scopeOf(target: EditorTarget): string {
  return target.kind === "dashboard"
    ? target.dashboardId
    : `observation:${target.observationId}:${target.blockId ?? "new"}`
}

export default function PanelEditor({
  target,
  spec,
  panel,
  version,
  readOnly,
  view,
  focusFirstQuery,
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
  const dashboard = target.kind === "dashboard" ? target : null
  const dashboardId = dashboard?.dashboardId ?? ""
  const adding = dashboard?.adding ?? false
  const propose = dashboard?.propose ?? false
  const latestVersion = dashboard?.latestVersion ?? version
  const [saveState, setSaveState] = useState<"idle" | "pending" | "saved">(
    "idle"
  )
  const observationId = target.kind === "block" ? target.observationId : ""
  const blockId = target.kind === "block" ? target.blockId : null
  const onBlock = dashboard === null
  const previewTarget = useMemo<PreviewTarget>(
    () =>
      onBlock
        ? { kind: "block", observationId, blockId }
        : { kind: "dashboard", dashboardId, version },
    [onBlock, observationId, blockId, dashboardId, version]
  )
  const datasources = useDatasources()
  const [draft, dispatch] = useReducer(draftReducer, undefined, () =>
    startDraft(version, panel)
  )
  const datasourceVariables = useMemo(
    () =>
      (spec.variables ?? []).filter(
        (variable): variable is DatasourceVariable =>
          variable.type === "datasource"
      ),
    [spec.variables]
  )
  const sourceTitle = useCallback(
    (panelId: string) => {
      const found = findPanel(spec, panelId)
      return found ? panelTitle(found.panel) : panelId
    },
    [spec]
  )
  const resolveDatasource = useMemo<ResolveDatasource>(
    () => (ref) => ({
      uid: resolveDatasourceRef(ref, selections ?? {}) ?? ref,
      type: datasourceRefType(ref, spec.variables ?? [], (uid) =>
        datasourceTypeOf(datasources.data, uid)
      ),
    }),
    [selections, spec.variables, datasources.data]
  )
  const changes = useMemo(
    () => panelChanges(draft.base, draft.panel),
    [draft.base, draft.panel]
  )
  const dirty = changes.length > 0
  const backup = useDraftBackup(scopeOf(target), draft, dirty)
  const [jsonValid, setJsonValid] = useState(true)
  const [metricsOpen, setMetricsOpen] = useState(false)
  const [activeRef, setActiveRef] = useState("")
  const [note, setNote] = useState("")
  const [noteOpen, setNoteOpen] = useState(false)
  const [problem, setProblem] = useState<Problem | null>(null)
  const [applying, setApplying] = useState(false)
  const validate = useSpecValidator()
  const editorRefs = useRef<(CodeEditorHandle | null)[]>([])
  const jsonGuard = useRef<LeaveGuard>(leaveNow)
  const noticeRef = useRef<HTMLDivElement>(null)
  const bodyRef = useRef<HTMLDivElement>(null)

  if (!dirty && version !== draft.baseVersion) {
    dispatch({ type: "rebase", version, panel })
  }

  useEffect(() => {
    onDirtyChange(dirty)
  }, [dirty, onDirtyChange])
  useEffect(() => () => onDirtyChange(false), [onDirtyChange])
  useEffect(() => {
    if (view !== "json") jsonGuard.current = leaveNow
  }, [view])

  useImperativeHandle(viewGuardRef, () => {
    const guarded = (leave: () => void, stay?: () => void) =>
      view === "json" ? jsonGuard.current(leave, stay) : leave()
    return {
      request: (next) => {
        if (next !== view) guarded(() => onViewChange(next))
      },
      guardLeave: guarded,
    }
  }, [view, onViewChange])

  const saving = saveState !== "idle"
  const newer = saving
    ? 0
    : Math.max(
        version > draft.baseVersion ? version : 0,
        latestVersion > draft.baseVersion ? latestVersion : 0,
        problem?.kind === "conflict" ? problem.latest : 0
      )
  const conflicted = problem?.kind === "conflict"
  const newestBody = useQuery({
    ...latestDashboardQuery(queryClient, dashboardId),
    enabled: dashboard !== null && newer > 0 && !readOnly,
  }).data
  const newestPanel =
    newestBody && newestBody.version.version >= newer
      ? findPanel(asDashboardSpec(newestBody.version.spec), draft.panel.id)
          ?.panel
      : undefined
  const newerAuthor = newerVersionAuthor(newestBody, newer)
  const overlaps = useMemo(
    () =>
      newestPanel && isEditablePanel(newestPanel)
        ? overlappingChanges(draft.base, draft.panel, newestPanel)
        : [],
    [newestPanel, draft.base, draft.panel]
  )
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
    if (issuesByQuery.size === 0) return
    bodyRef.current
      ?.querySelector('[role="alert"]')
      ?.scrollIntoView({ block: "nearest" })
  }, [issuesByQuery])

  const [runBlocked, setRunBlocked] = useState(false)
  const [inlineErrorRefs, setInlineErrorRefs] = useState<ReadonlySet<string>>(
    () => new Set()
  )
  const setInlineError = useCallback((ref: string, shown: boolean) => {
    setInlineErrorRefs((current) => {
      if (current.has(ref) === shown) return current
      const next = new Set(current)
      if (shown) next.add(ref)
      else next.delete(ref)
      return next
    })
  }, [])
  const showDraftTitle = useDraftTitles((state) => state.show)
  const draftPanelId = draft.panel.id
  const draftTitle = draft.panel.title
  useEffect(() => {
    showDraftTitle(draftPanelId, draftTitle)
  }, [showDraftTitle, draftPanelId, draftTitle])
  useEffect(
    () => () => showDraftTitle(draftPanelId, undefined),
    [showDraftTitle, draftPanelId]
  )
  const conflictTitles = useMemo(() => panelTitles(spec), [spec])
  const current = draft.panel
  const jsonBlocked = view === "json" && !jsonValid
  const deleted = dashboard?.deleted ?? false
  const blockedText = saveBlockedText({ deleted, jsonBlocked, runBlocked })
  const titleOk = current.title === undefined || current.title.trim() !== ""
  const queriesOk = isDataPanel(current)
    ? current.queries.every(
        (query) =>
          !isPanelQuery(query) ||
          (query.query.trim() !== "" &&
            [...query.query].length <= maxQueryLength)
      )
    : current.type !== "text" || [...current.content].length <= maxContentLength
  const locked = readOnly || saving || applying
  const canSave =
    validate !== undefined &&
    dirty &&
    !locked &&
    blockedText === null &&
    queriesOk &&
    titleOk

  function failed(
    error: unknown,
    pointer: string,
    normalised: EditablePanel,
    retryOnLatest: (() => void) | null
  ) {
    if (error instanceof ApiError) {
      if (error.status === 409 && error.latestVersion !== undefined) {
        setProblem({
          kind: "conflict",
          latest: error.latestVersion,
          conflicts: error.conflicts,
        })
        return
      }
      if (error.status === 409 && dashboard === null) {
        setProblem(null)
        return
      }
      if (
        error.status === 422 &&
        retryOnLatest !== null &&
        onlyGridOverlap(error.issues)
      ) {
        retryOnLatest()
        return
      }
      if (
        error.status === 422 &&
        (dashboard !== null || error.issues.length > 0)
      ) {
        if (error.issues.length > 0 && error.issues.every(isTransient)) {
          setProblem({
            kind: "transient",
            lines: transientLines(
              error.issues,
              pointer,
              querySources(normalised, (ref) => resolveDatasource(ref).uid)
            ),
          })
          return
        }
        const [firstIssue] = error.issues
        if (firstIssue !== undefined && onlyEmptyResult(error.issues)) {
          setProblem({ kind: "empty", message: firstIssue.message })
          return
        }
        setProblem({
          kind: "rejected",
          issues: error.issues,
          pointer,
          refs: isDataPanel(normalised) ? normalised.queries.map(queryRef) : [],
        })
        return
      }
    }
    setProblem({
      kind: "failed",
      message: error instanceof Error ? error.message : "The save failed.",
    })
  }

  function submit(
    source: Dashboard,
    parent: number,
    panelToSave: EditablePanel,
    acceptEmpty = false,
    replaced = false
  ) {
    if (!validate) return
    const entry = findPanel(source, panelToSave.id)
    if (!entry) {
      setProblem({ kind: "gone" })
      return
    }
    const normalised = normalisePanel(panelToSave)
    const parsed = parsePanelJson(
      JSON.stringify(normalised),
      source,
      normalised.id,
      validate
    )
    if (!parsed.ok) {
      setProblem(
        parent === draft.baseVersion
          ? { kind: "invalid", issues: parsed.issues }
          : { kind: "clash", version: parent, issues: parsed.issues }
      )
      return
    }
    setProblem(null)
    setSaveState("pending")
    const title = normalised.title || normalised.id
    const saved =
      target.kind === "block"
        ? target.save(parsed.panel)
        : target.save({
            patch: { upsert: [{ ...parsed.panel }] },
            summary:
              note.trim() ||
              (adding
                ? `Add "${title}"`
                : defaultSaveSummary(
                    changes,
                    entry.panel.title || entry.panel.id
                  )),
            parent_version: parent,
            propose,
            acceptEmpty,
          })
    saved.then(
      () => {
        setSaveState("saved")
        onSaved()
      },
      (error: unknown) => {
        setSaveState("idle")
        failed(
          error,
          dashboard === null ? blockIssuePointer : entry.pointer,
          normalised,
          adding && !replaced
            ? () => void applyToLatest(acceptEmpty, true)
            : null
        )
      }
    )
  }

  async function withLatestPanel(
    onLatest: (latest: {
      spec: Dashboard
      version: number
      panel: EditablePanel
    }) => void
  ) {
    if (dashboard === null) {
      setProblem(null)
      onLatest({ spec, version, panel })
      return
    }
    setApplying(true)
    setProblem(null)
    try {
      const latest = await queryClient.fetchQuery({
        ...latestDashboardQuery(queryClient, dashboardId),
        staleTime: 0,
      })
      const savedSpec = asDashboardSpec(latest.version.spec)
      const latestSpec =
        adding &&
        isDataPanel(draft.base) &&
        findPanel(savedSpec, draft.base.id) === undefined
          ? withNewPanel(savedSpec, draft.base)
          : savedSpec
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
      onLatest({
        spec: latestSpec,
        version: latest.version.version,
        panel: entry.panel,
      })
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

  function applyToLatest(acceptEmpty = false, replaced = false) {
    return withLatestPanel(({ spec: latestSpec, version: latestOf, panel }) =>
      submit(
        latestSpec,
        latestOf,
        rebasePanel(draft.base, draft.panel, panel),
        acceptEmpty,
        replaced
      )
    )
  }

  function reload() {
    return withLatestPanel(({ version: latestOf, panel }) =>
      dispatch({ type: "rebase", version: latestOf, panel })
    )
  }

  function saveDraft() {
    if (newer > 0) void applyToLatest()
    else submit(spec, draft.baseVersion, draft.panel)
  }

  function saveAnyway() {
    if (newer > 0) void applyToLatest(true)
    else submit(spec, draft.baseVersion, draft.panel, true)
  }

  const registerJsonGuard = useCallback((guard: LeaveGuard) => {
    jsonGuard.current = guard
  }, [])

  const outcome =
    target.kind === "block"
      ? target.outcome
      : propose
        ? "An editor merges your change"
        : null
  const footerSummary =
    blockedText ??
    (outcome === null
      ? changeSummary(changes)
      : `${changeSummary(changes)}. ${outcome}`)
  let saveLabel = `Save as version ${latestVersion + 1}`
  if (target.kind === "block") saveLabel = target.saveLabel
  else if (propose) saveLabel = "Propose change"
  else if (newer > 0) saveLabel = `Save my changes as version ${newer + 1}`

  const data = isDataPanel(current) ? current : null
  const activeQuery = data ? activeQueryIndex(data.queries, activeRef) : 0
  const active = data?.queries[activeQuery]
  const activeDatasource = resolveDatasource(
    (active && isPanelQuery(active) ? active.datasource : undefined) ??
      data?.datasource ??
      ""
  )

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ScrollPane
        ref={bodyRef}
        className="@container flex flex-1 flex-col gap-4 px-4 py-3.5"
      >
        {backup.kept ? (
          <Alert>
            <AlertTitle>Your unsaved changes were kept.</AlertTitle>
            <AlertDescription>
              <p>They were still open when your session ended.</p>
              <div className="mt-2 flex gap-2">
                <Button
                  size="xs"
                  onClick={() => {
                    const kept = backup.kept
                    backup.drop()
                    if (kept) {
                      dispatch({
                        type: "replace",
                        panel: restoreBackup(kept, draft.panel),
                      })
                    }
                  }}
                >
                  Restore my changes
                </Button>
                <Button size="xs" variant="outline" onClick={backup.drop}>
                  Discard them
                </Button>
              </div>
            </AlertDescription>
          </Alert>
        ) : null}
        <PreviewSection
          target={previewTarget}
          spec={spec}
          baseVersion={draft.baseVersion}
          base={draft.base}
          draft={draft.panel}
          timeRange={timeRange}
          selections={selections}
          onSaveBlockedChange={setRunBlocked}
          inlineErrorRefs={inlineErrorRefs}
        />
        {dashboard === null ? null : (
          <div className={view === "editor" ? "contents" : "hidden"}>
            <AskAgent
              dashboardId={dashboardId}
              version={draft.baseVersion}
              draft={draft.panel}
              dispatch={dispatch}
              blockedReason={
                readOnly ? readOnlyAskReason : locked ? savingText : null
              }
            />
          </div>
        )}
        {view === "editor" ? (
          <>
            <div className="grid grid-cols-[minmax(0,1fr)_220px] items-start gap-5 @max-[34rem]:grid-cols-1">
              <div className="flex min-w-0 flex-col gap-3">
                {data && !reusesPanel(data) ? (
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Label htmlFor={datasourceId} className="text-[13px]">
                        Datasource
                      </Label>
                      <DatasourceSelect
                        id={datasourceId}
                        value={data.datasource ?? ""}
                        variables={datasourceVariables}
                        disabled={locked}
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
                  readOnly={locked}
                  datasourceVariables={datasourceVariables}
                  resolveDatasource={resolveDatasource}
                  issuesByQuery={issuesByQuery}
                  sourceTitle={sourceTitle}
                  onActiveChange={setActiveRef}
                  onRemoved={(ref) =>
                    setProblem((current) => withoutQuery(current, ref))
                  }
                  onInlineErrorChange={setInlineError}
                  focusFirst={focusFirstQuery}
                  editorRefs={editorRefs}
                  beforeAdd={
                    metricsOpen && data ? (
                      <MetricsBrowser
                        datasourceUid={activeDatasource.uid}
                        datasourceType={activeDatasource.type}
                        onInsert={(text) => {
                          if (!locked) {
                            editorRefs.current[activeQuery]?.insertAtCursor(
                              text
                            )
                          }
                        }}
                      />
                    ) : null
                  }
                />
              </div>
              <SettingsColumn
                draft={draft.panel}
                dispatch={dispatch}
                readOnly={locked}
                onOpenJson={() => onViewChange("json")}
              />
            </div>
          </>
        ) : (
          <JsonView
            spec={spec}
            draft={draft.panel}
            dispatch={dispatch}
            readOnly={locked}
            onValidityChange={setJsonValid}
            onLeaveRequest={registerJsonGuard}
          />
        )}
      </ScrollPane>
      {readOnly || (newer === 0 && !showsProblem(problem)) ? null : (
        <div className="flex max-h-[40%] flex-col gap-2 overflow-auto px-4 pb-3">
          {newer > 0 ? (
            <Alert
              ref={noticeRef}
              tabIndex={-1}
              className="outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring"
            >
              {problem?.kind === "conflict" ? (
                <MergeConflicts
                  conflicts={problem.conflicts}
                  titles={conflictTitles}
                  latest={problem.latest}
                  onReload={() => void reload()}
                />
              ) : (
                <NewerVersionNotice
                  version={newestBody?.version.version ?? newer}
                  author={newerAuthor}
                  overlaps={overlaps}
                />
              )}
            </Alert>
          ) : null}
          {problem ? (
            <SaveProblem
              problem={problem}
              panelId={current.id}
              onRetry={saveDraft}
              onSaveAnyway={saveAnyway}
            />
          ) : null}
        </div>
      )}
      <EditorFooter
        summary={footerSummary}
        noteOpen={noteOpen}
        note={note}
        onNoteOpen={dashboard === null ? undefined : () => setNoteOpen(true)}
        onNoteChange={setNote}
        saving={saving || applying}
        canSave={canSave}
        saveLabel={saveLabel}
        onSave={saveDraft}
        onDiscard={onDiscard}
        readOnly={readOnly}
        onOpenLatest={onOpenLatest}
        status={saving || applying ? savingText : null}
      />
    </div>
  )
}
