import { useCallback, useEffect, useId, useRef, useState } from "react"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"

import { useInputMode } from "@/ask/input-mode"
import { CopyButton } from "@/components/copy-button"
import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import {
  rememberDatasource,
  rememberedDatasource,
} from "@/datasources/datasource-memory"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { liveRegionText } from "@/lib/live-region"
import { roleAtLeast } from "@/lib/roles"
import { useAgentInfo } from "@/observe/queries"
import { AddToObservationPanel } from "@/observations/add-to-observation-panel"
import type { LogSelection } from "@/panels/logs/logs-panel"

import { logsDraft } from "@/saved-searches/draft"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { SavedSearchesMenu } from "@/saved-searches/saved-searches-menu"

import { logsPanelFor } from "./add-panel"
import { AddToDashboardPanel } from "./add-to-dashboard-panel"
import { clickhouseLogsTemplate } from "./context-query"
import { DatasourceControl } from "./datasource-control"
import { eligibleDatasources } from "./datasource-picker"
import { DetailDrawer } from "./detail-drawer"
import { FieldsAside } from "./fields-aside"
import { FilterPills } from "./filter-pills"
import { rowCount } from "./frames"
import { QueryBar } from "./query-bar"
import { ResultSkeleton, ResultsSection } from "./results-section"
import {
  logsDatasourceTypes,
  logsTimeRange,
  searchAfter,
  type LogsSearch,
} from "./search"
import { useLiveTail } from "./use-live-tail"
import { VolumeStrip } from "./volume-strip"
import {
  LogsWorkspaceContext,
  useLogsResults,
  type DetailTab,
  type LogsWorkspace,
} from "./workspace"

const routeId = "/_authed/_shell/logs"
const description =
  "Search one datasource in its own query language, narrow by field, then jump to the trace."

type Update = LogsWorkspace["update"]

function saveHint(savable: ReturnType<typeof logsDraft>): string | undefined {
  if (!("blocked" in savable)) return undefined
  return savable.blocked === "no-query"
    ? "Run a query first."
    : "Filters are not applied: switch the query to KQL."
}

function useLogsSearch() {
  const search = useSearch({ from: routeId })
  const navigate = useNavigate({ from: "/logs" })
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

function useDefaultDatasource(
  current: DatasourceStatus | undefined,
  eligible: readonly DatasourceStatus[]
) {
  const navigate = useNavigate()
  const fallback =
    current === undefined ? rememberedDatasource("logs", eligible) : undefined
  const uid = current?.uid
  useEffect(() => {
    if (fallback === undefined) return
    void navigate({
      to: ".",
      search: (state: Record<string, unknown>) => ({ ...state, ds: fallback }),
      replace: true,
    })
  }, [fallback, navigate])
  useEffect(() => {
    if (uid !== undefined) rememberDatasource("logs", uid)
  }, [uid])
}

export function LogsPage() {
  const { search, update } = useLogsSearch()
  const datasources = useDatasources()
  const { principal } = useRouteContext({ from: "/_authed" })
  const eligible = eligibleDatasources(datasources.data, logsDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  useDefaultDatasource(datasource, eligible)

  if (datasources.isPending) {
    return (
      <>
        <PageHeader title="Logs" />
        <ResultSkeleton />
      </>
    )
  }
  if (eligible.length === 0) {
    return (
      <>
        <PageHeader title="Logs" />
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
  if (datasource === undefined) return <PageHeader title="Logs" />
  return (
    <LogsWorkspaceView
      key={datasource.uid}
      datasource={datasource}
      datasources={eligible}
      search={search}
      update={update}
    />
  )
}

function LogsWorkspaceView({
  datasource,
  datasources,
  search,
  update,
}: {
  datasource: DatasourceStatus
  datasources: DatasourceStatus[]
  search: LogsSearch
  update: Update
}) {
  const notRunId = useId()
  const uid = datasource.uid
  const type = datasource.type
  const range = logsTimeRange(search)
  const q = search.q
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
  const resultsScope = `${q ?? ""}|${range.from}|${range.to}`
  const current = held?.scope === resultsScope ? held : null
  const selection = current?.selection ?? null
  const detailTab = current?.tab ?? "table"
  const [adding, setAdding] = useState(false)
  const [addingToObservation, setAddingToObservation] = useState(false)
  const [saving, setSaving] = useState(false)
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
    search,
    range,
    update,
  })
  const liveTail = useLiveTail({
    uid,
    query: q ?? "",
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
    datasources,
    search,
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
    focusQuery: () => setFocusTick((tick) => tick + 1),
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
      <div className="flex h-full min-h-0 flex-col gap-3">
        <PageHeader title="Logs" description={description} />
        <div className="flex flex-wrap items-center gap-2">
          <QueryBar.ModeSwitch />
          <div className="ml-auto flex flex-wrap items-center gap-2">
            <DatasourceControl />
            <TimeRangeControl
              value={range}
              onChange={(next) => {
                setTailing(false)
                setTailShown(false)
                update(next)
              }}
            />
            <Button
              type="button"
              variant="outline"
              aria-pressed={tailOn}
              disabled={q === undefined}
              title={q === undefined ? "Run a query first." : undefined}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => (tailOn ? stopTail() : startTail())}
            >
              {tailOn ? "Stop live tail" : "Live tail"}
            </Button>
            <CopyButton
              text={window.location.href}
              label="Copy link"
              variant="outline"
              icon={false}
            >
              Copy link
            </CopyButton>
            <SavedSearchesMenu kind="logs" />
            <Button
              ref={saveTrigger}
              type="button"
              variant="outline"
              disabled={"blocked" in savable}
              title={saveHint(savable)}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => setSaving(true)}
            >
              Save search
            </Button>
            <Button
              ref={addTrigger}
              type="button"
              variant="outline"
              disabled={q === undefined}
              title={q === undefined ? "Run a query first." : undefined}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => {
                selectLine(null)
                setAddingToObservation(false)
                setAdding(true)
              }}
            >
              Add to dashboard
            </Button>
            <Button
              ref={observationTrigger}
              type="button"
              variant="outline"
              disabled={q === undefined}
              title={q === undefined ? "Run a query first." : undefined}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => {
                selectLine(null)
                setAdding(false)
                setAddingToObservation(true)
              }}
            >
              Add to observation
            </Button>
            {q === undefined ? (
              <span id={notRunId} className="sr-only">
                Run a query first.
              </span>
            ) : null}
          </div>
        </div>
        <QueryBar />
        <FilterPills />
        {q !== undefined && !tailShown ? (
          <VolumeStrip uid={uid} query={q} from={range.from} to={range.to} />
        ) : null}
        <div className="flex min-h-96 min-w-0 flex-1 gap-3">
          <FieldsAside />
          <ResultsSection />
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
      {"draft" in savable ? (
        <SaveSearchDialog
          draft={savable.draft}
          open={saving}
          onOpenChange={setSaving}
          finalFocus={() => saveTrigger.current ?? true}
        />
      ) : null}
      {adding && q !== undefined ? (
        <AddToDashboardPanel
          datasource={uid}
          query={q}
          onClose={() => setAdding(false)}
          trigger={addTrigger}
        />
      ) : null}
      {addingToObservation && q !== undefined ? (
        <AddToObservationPanel
          panel={logsPanelFor({
            id: "logs",
            title: "Logs",
            datasource: uid,
            query: q,
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
