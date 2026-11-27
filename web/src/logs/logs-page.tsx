import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"
import { LayoutDashboardIcon, NotebookTextIcon } from "lucide-react"

import { loadProblemText } from "@/api/errors"
import { useInputMode } from "@/ask/input-mode"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { SourceTitle } from "@/datasources/source-title"
import { UnknownSource } from "@/datasources/unknown-source"
import { useExploreSources } from "@/datasources/use-sources"
import { useDocumentTitle } from "@/lib/document-title"
import { liveRegionText } from "@/lib/live-region"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import type { LogSelection } from "@/panels/logs/logs-panel"

import { logsDraft } from "@/saved-searches/draft"
import { OpenSavedSearch } from "@/saved-searches/open-saved-search"
import { ExploreActions } from "@/saved-searches/explore-actions"
import { useLeaveAskOnSavedOpen } from "@/saved-searches/saved-open"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { exploreRange } from "@/lib/time"

import { logsPanelFor } from "./add-panel"
import { AddToDashboardPanel } from "./add-to-dashboard-panel"
import { openingQuery } from "./context-query"
import { DetailDrawer } from "./detail-drawer"
import { FieldsAside, FieldsDrawer } from "./fields-aside"
import { FilterPills } from "./filter-pills"
import { rowCount } from "./frames"
import { hiddenLevels, levelsParam, parseLevels, pickLevel } from "./levels"
import { LogsShortcutsDialog } from "./logs-shortcuts-dialog"
import { applyPills, decodePills } from "./pills"
import { unreadableKql } from "./query-problem"
import { QueryBar } from "./query-bar"
import { ResultSkeleton, ResultsSection } from "./results-section"
import { runPatch, searchAfter, type LogsSearch } from "./search"
import { useLiveTail } from "./use-live-tail"
import { useLogsShortcuts } from "./use-logs-shortcuts"
import { isPlainResult } from "./plain-table"
import { VolumeStrip } from "./volume-strip"
import {
  LogsWorkspaceContext,
  useLogsResults,
  type DetailTab,
  type LogsWorkspace,
} from "./workspace"

const routeId = "/_authed/_shell/logs/$datasourceUid"

type Update = LogsWorkspace["update"]

function useLogsSearch() {
  const search = useSearch({ from: routeId })
  const navigate = useNavigate({ from: "/logs/$datasourceUid" })
  const update = useCallback<Update>(
    (next, options) =>
      void navigate({
        search: (state) => searchAfter(state, next),
        replace: options?.replace,
      }),
    [navigate]
  )
  return { search, update }
}

export function LogsPage({ uid }: { uid: string }) {
  const { search, update } = useLogsSearch()
  const { sources, isPending, isError, error, refetch } =
    useExploreSources("logs")
  const { principal } = useRouteContext({ from: "/_authed" })
  const datasource = sources.find((candidate) => candidate.uid === uid)
  const rememberedUid = datasource?.uid
  useDocumentTitle(
    datasource
      ? `Logs / ${datasource.name} - SlashObserve`
      : "Logs - SlashObserve"
  )
  useEffect(() => {
    if (rememberedUid !== undefined) {
      useSourceMemory.getState().remember("logs", rememberedUid)
    }
  }, [rememberedUid])

  if (isPending) {
    return (
      <>
        <h1 className="sr-only">Logs</h1>
        <ResultSkeleton />
      </>
    )
  }
  if (isError) {
    return (
      <>
        <h1 className="sr-only">Logs</h1>
        <LoadProblem
          title="Could not load the datasources"
          detail={loadProblemText(error)}
          retrying={false}
          onRetry={() => void refetch()}
        />
      </>
    )
  }
  if (sources.length === 0) {
    return (
      <>
        <h1 className="sr-only">Logs</h1>
        <p className="text-[13px] text-muted-foreground">
          No logs datasource is configured. An Admin adds an Elasticsearch,
          OpenSearch, Loki or ClickHouse datasource in the datasources file.
        </p>
        {roleAtLeast(principal.role, "admin") ? (
          <Link
            to="/datasources"
            className="mt-2 inline-block text-[13px] underline underline-offset-3"
          >
            Open datasources
          </Link>
        ) : null}
      </>
    )
  }
  if (datasource === undefined) {
    return <UnknownSource kind="logs" uid={uid} />
  }
  return (
    <LogsWorkspaceView
      key={datasource.uid}
      datasource={datasource}
      search={search}
      update={update}
    />
  )
}

const notRunText = "Run a query first."

function saveHint(savable: ReturnType<typeof logsDraft>): string | undefined {
  if (!("blocked" in savable)) return undefined
  return savable.blocked === "no-query"
    ? notRunText
    : "Filters are not applied: switch the query to KQL."
}

function LogsWorkspaceView({
  datasource,
  search,
  update,
}: {
  datasource: DatasourceStatus
  search: LogsSearch
  update: Update
}) {
  const notRunId = useId()
  const uid = datasource.uid
  const type = datasource.type
  const range = exploreRange(search)
  const q = search.q
  const filters = search.f
  const effectiveQuery = useMemo(
    () =>
      q === undefined || unreadableKql(q, type) !== null
        ? undefined
        : (applyPills(q, type, decodePills(filters)) ?? q),
    [q, type, filters]
  )
  const opening = openingQuery(type)
  const [draft, setDraft] = useState(() => q ?? opening ?? "")
  const [seenQuery, setSeenQuery] = useState(q)
  if (seenQuery !== q) {
    setSeenQuery(q)
    if (q !== undefined) setDraft(q)
  }
  const shownLevels = useMemo(() => parseLevels(search.levels), [search.levels])
  const hiddenLevelSet = useMemo(() => hiddenLevels(shownLevels), [shownLevels])
  const [tailSession, setTailSession] = useState(0)
  const [tailing, setTailing] = useState(false)
  const [tailShown, setTailShown] = useState(false)
  const [held, setHeld] = useState<{
    selection: LogSelection
    tab: DetailTab
    scope: string
  } | null>(null)
  const drawerId = useId()
  const resultsScope = `${effectiveQuery ?? ""}|${range.from}|${range.to}`
  const [tailScope, setTailScope] = useState(resultsScope)
  if (tailScope !== resultsScope) {
    setTailScope(resultsScope)
    setTailing(false)
    setTailShown(false)
  }
  const current = held?.scope === resultsScope ? held : null
  const selection = current?.selection ?? null
  const detailTab = current?.tab ?? "table"
  const [adding, setAdding] = useState(false)
  const [addingToObservation, setAddingToObservation] = useState(false)
  const [saving, setSaving] = useState(false)
  const [helpOpen, setHelpOpen] = useState(false)
  const saveTrigger = useRef<HTMLButtonElement>(null)
  const savable = logsDraft({ uid, type, search, range })
  const [labelsOpen, setLabelsOpen] = useState(false)
  const [announcement, setAnnouncement] = useState({ text: "", count: 0 })
  const [focusTick, setFocusTick] = useState(0)
  const queryFocus = useRef<() => void>(() => {})
  const selectionTrigger = useRef<HTMLElement | null>(null)
  const addTrigger = useRef<HTMLButtonElement>(null)
  const observationTrigger = useRef<HTMLButtonElement>(null)
  const agentAvailable = useAgentInfo().data?.available === true
  const [mode, setMode] = useInputMode({
    agentAvailable,
    hasQuery: q !== undefined,
    hasAsk: search.ask !== undefined,
  })
  useLeaveAskOnSavedOpen(mode, setMode)

  const { results, paging } = useLogsResults({
    datasource,
    query: effectiveQuery,
    search,
    range,
    update,
  })
  const liveTail = useLiveTail({
    uid,
    query: effectiveQuery ?? "",
    seed: paging.page === 1 ? (results.data?.frames ?? []) : [],
    running: tailing,
    session: tailSession,
    onFailed: () => setTailing(false),
  })

  useEffect(() => {
    if (opening !== undefined && q === undefined) {
      update({ q: opening }, { replace: true })
    }
  }, [opening, q, update])

  useEffect(() => {
    if (focusTick > 0) queryFocus.current()
  }, [focusTick])

  const focusQuery = () => setFocusTick((tick) => tick + 1)
  useLogsShortcuts({ focusQuery, openHelp: () => setHelpOpen(true) })

  const registerQueryFocus = useCallback((focus: () => void) => {
    queryFocus.current = focus
  }, [])

  const tailFrames = liveTail.buffer?.frames
  const shownLines = tailShown
    ? tailFrames
      ? rowCount(tailFrames)
      : 0
    : results.data
      ? rowCount(results.data.frames)
      : 0
  function announce(text: string) {
    setAnnouncement((current) => ({ text, count: current.count + 1 }))
  }

  function stopTail() {
    if (!tailing) return
    setTailing(false)
    announce(`Live tail stopped, ${shownLines.toLocaleString("en")} lines`)
  }

  function startTail() {
    setTailSession((session) => session + 1)
    setTailing(true)
    setTailShown(true)
    announce("Live tail on")
  }

  function run(raw?: string, asked?: string, alongside?: Partial<LogsSearch>) {
    const text = (raw ?? draft).trim()
    if (text === "") return
    setTailing(false)
    setTailShown(false)
    const next = runPatch({ text, current: search, asked, alongside })
    if (Object.keys(next).length === 0) void results.refetch()
    else update(next)
  }

  function selectLine(next: LogSelection | null, tab?: DetailTab) {
    if (next === null) {
      setHeld(null)
      return
    }
    if (selection === null) {
      selectionTrigger.current =
        document.activeElement instanceof HTMLElement
          ? document.activeElement
          : null
    }
    setAdding(false)
    setAddingToObservation(false)
    setHeld({ selection: next, tab: tab ?? detailTab, scope: resultsScope })
  }

  function setDetailTab(tab: DetailTab) {
    setHeld((current) => (current === null ? null : { ...current, tab }))
  }

  const workspace: LogsWorkspace = {
    datasource,
    search,
    effectiveQuery,
    update,
    draft,
    setDraft,
    run,
    range,
    results,
    paging,
    tail: {
      ...liveTail,
      on: tailing,
      shown: tailShown,
      start: startTail,
      stop: stopTail,
    },
    mode,
    setMode,
    announce,
    focusQuery,
    registerQueryFocus,
    labels: { open: labelsOpen, setOpen: setLabelsOpen },
    levels: {
      shown: shownLevels,
      hidden: hiddenLevelSet,
      pick: (kind, additive) =>
        update(
          { levels: levelsParam(pickLevel(shownLevels, kind, additive)) },
          { replace: true }
        ),
      clear: () => update({ levels: undefined }, { replace: true }),
    },
    detail: {
      selection,
      tab: detailTab,
      drawerId,
      select: selectLine,
      setTab: setDetailTab,
    },
  }

  return (
    <LogsWorkspaceContext value={workspace}>
      <ExploreActions
        kind="logs"
        range={range}
        onRangeChange={(next) => {
          setTailing(false)
          setTailShown(false)
          update(next)
        }}
        tail={{
          on: tailing,
          blocked: q === undefined ? notRunText : undefined,
          toggle: () => (tailing ? stopTail() : startTail()),
        }}
        saveBlocked={saveHint(savable)}
        describedBy={q === undefined ? notRunId : undefined}
        saveTrigger={saveTrigger}
        onSave={() => setSaving(true)}
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <h1 className="sr-only">Logs in {datasource.name}</h1>
        <SourceTitle kind="logs" source={datasource} />
        <OpenSavedSearch
          kind="logs"
          uid={uid}
          savedId={search.saved}
          draft={"draft" in savable ? savable.draft : undefined}
          onClose={() => update({ saved: undefined }, { replace: true })}
        />
        {q === undefined ? (
          <span id={notRunId} className="sr-only">
            Run a query first.
          </span>
        ) : null}
        <section
          aria-label="Query"
          className="flex flex-col rounded-card border bg-card"
        >
          <div className="px-4 pt-3">
            <QueryBar.ModeSwitch />
          </div>
          <QueryBar filters={<FilterPills />} />
        </section>
        {effectiveQuery !== undefined &&
        !tailShown &&
        (results.data === undefined || !isPlainResult(results.data)) ? (
          <VolumeStrip
            uid={uid}
            query={effectiveQuery}
            from={range.from}
            to={range.to}
          />
        ) : null}
        <div className="flex min-h-[34rem] min-w-0 flex-1 overflow-hidden rounded-card border bg-card narrow:min-h-[48rem]">
          <FieldsAside />
          <ResultsSection
            actions={
              <>
                <FieldsDrawer />
                <Button
                  ref={addTrigger}
                  type="button"
                  variant="ghost"
                  size="sm"
                  className="narrow:h-11"
                  disabled={q === undefined}
                  title={q === undefined ? "Run a query first." : undefined}
                  aria-describedby={q === undefined ? notRunId : undefined}
                  onClick={() => {
                    selectLine(null)
                    setAddingToObservation(false)
                    setAdding(true)
                  }}
                >
                  <LayoutDashboardIcon aria-hidden="true" />
                  Add to dashboard
                </Button>
                <Button
                  ref={observationTrigger}
                  type="button"
                  variant="outline"
                  size="sm"
                  className="narrow:h-11"
                  disabled={q === undefined}
                  title={q === undefined ? "Run a query first." : undefined}
                  aria-describedby={q === undefined ? notRunId : undefined}
                  onClick={() => {
                    selectLine(null)
                    setAdding(false)
                    setAddingToObservation(true)
                  }}
                >
                  <NotebookTextIcon aria-hidden="true" />
                  Add to observation
                </Button>
              </>
            }
          />
        </div>
        <span role="status" className="sr-only">
          {liveRegionText(announcement.text, announcement.count)}
        </span>
      </div>
      {selection ? (
        <DetailDrawer
          selection={selection}
          tab={detailTab}
          onTabChange={setDetailTab}
          onClose={() => selectLine(null)}
          trigger={selectionTrigger}
          id={drawerId}
        />
      ) : null}
      <LogsShortcutsDialog open={helpOpen} onOpenChange={setHelpOpen} />
      {"draft" in savable ? (
        <SaveSearchDialog
          draft={savable.draft}
          open={saving}
          onOpenChange={setSaving}
          finalFocus={() => saveTrigger.current ?? true}
        />
      ) : null}
      {adding && effectiveQuery !== undefined ? (
        <AddToDashboardPanel
          datasource={uid}
          query={effectiveQuery}
          onClose={() => setAdding(false)}
          trigger={addTrigger}
        />
      ) : null}
      {addingToObservation && effectiveQuery !== undefined ? (
        <AddToObservationPanel
          panel={logsPanelFor({
            id: "logs",
            title: "Logs",
            datasource: uid,
            query: effectiveQuery,
            y: 0,
          })}
          from={range.from}
          to={range.to}
          defaultObservationId={search.observation}
          onClose={() => setAddingToObservation(false)}
          trigger={observationTrigger}
        />
      ) : null}
    </LogsWorkspaceContext>
  )
}
