import { useCallback, useEffect, useId, useMemo, useRef, useState } from "react"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"
import { LayoutDashboardIcon, NotebookTextIcon } from "lucide-react"

import { useInputMode } from "@/ask/input-mode"
import { LoadProblem } from "@/components/load-problem"
import { Button } from "@/components/ui/button"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { SourceTitle } from "@/datasources/source-title"
import { useExploreSources } from "@/datasources/use-sources"
import { useDocumentTitle } from "@/lib/document-title"
import { liveRegionText } from "@/lib/live-region"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import type { LogSelection } from "@/panels/logs/logs-panel"

import { logsDraft } from "@/saved-searches/draft"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { loadProblemText } from "@/service-accounts/queries"

import { logsPanelFor } from "./add-panel"
import { AddToDashboardPanel } from "./add-to-dashboard-panel"
import { clickhouseLogsTemplate } from "./context-query"
import { DetailDrawer } from "./detail-drawer"
import { FieldsAside } from "./fields-aside"
import { FilterPills } from "./filter-pills"
import { rowCount } from "./frames"
import { LogsShortcutsDialog } from "./logs-shortcuts-dialog"
import { LogsPageActions } from "./page-actions"
import { applyPills, decodePills } from "./pills"
import { QueryBar } from "./query-bar"
import { ResultSkeleton, ResultsSection } from "./results-section"
import { logsTimeRange, searchAfter, type LogsSearch } from "./search"
import { useLiveTail } from "./use-live-tail"
import { useLogsShortcuts } from "./use-logs-shortcuts"
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
    return (
      <>
        <h1 className="sr-only">Logs</h1>
        <p className="text-[13px] text-muted-foreground">
          No log source has the uid {uid}.
        </p>
        <Link
          to="/logs"
          className="mt-2 inline-block text-[13px] underline underline-offset-3"
        >
          Open Logs
        </Link>
      </>
    )
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
  const range = logsTimeRange(search)
  const q = search.q
  const filters = search.f
  const effectiveQuery = useMemo(
    () =>
      q === undefined
        ? undefined
        : (applyPills(q, type, decodePills(filters)) ?? q),
    [q, type, filters]
  )
  const [draft, setDraft] = useState(
    () => q ?? (type === "clickhouse" ? clickhouseLogsTemplate : "")
  )
  const [seenQuery, setSeenQuery] = useState(q)
  if (seenQuery !== q) {
    setSeenQuery(q)
    if (q !== undefined) setDraft(q)
  }
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
  const [labelsOpen, setLabelsOpen] = useState(
    type === "loki" && q === undefined
  )
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
  })

  useEffect(() => {
    if (type === "clickhouse" && q === undefined) {
      update({ q: clickhouseLogsTemplate }, { replace: true })
    }
  }, [type, q, update])

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
  const tailOn = tailing && liveTail.error === null

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

  function run(raw?: string, asked?: string) {
    const text = (raw ?? draft).trim()
    if (text === "") return
    setTailing(false)
    setTailShown(false)
    if (text === q) void results.refetch()
    if (text !== q || (asked !== undefined && asked !== search.ask)) {
      update(asked === undefined ? { q: text } : { q: text, ask: asked })
    }
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
      on: tailOn,
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
      <LogsPageActions
        range={range}
        onRangeChange={(next) => {
          setTailing(false)
          setTailShown(false)
          update(next)
        }}
        tail={{ on: tailOn, start: startTail, stop: stopTail }}
        queryRun={q !== undefined}
        notRunId={notRunId}
        savable={savable}
        saveTrigger={saveTrigger}
        onSave={() => setSaving(true)}
      />
      <div className="flex h-full min-h-0 flex-col gap-3">
        <h1 className="sr-only">Logs in {datasource.name}</h1>
        <SourceTitle kind="logs" source={datasource} />
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
        {effectiveQuery !== undefined && !tailShown ? (
          <VolumeStrip
            uid={uid}
            query={effectiveQuery}
            from={range.from}
            to={range.to}
          />
        ) : null}
        <div className="flex min-h-96 min-w-0 flex-1 overflow-hidden rounded-card border bg-card">
          <FieldsAside />
          <ResultsSection
            actions={
              <>
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
