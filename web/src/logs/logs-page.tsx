import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { liveRegionText } from "@/lib/live-region"
import { roleAtLeast } from "@/lib/roles"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import { labelNamesQuery } from "@/panel-editor/queries"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"

import { AddToDashboardPanel } from "./add-to-dashboard-panel"
import { clickhouseLogsTemplate } from "./context-query"
import { ContextPanel } from "./context-panel"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "./datasource-picker"
import { exploreErrorText, exploreQuery, offlineText } from "./explore"
import { rowCount } from "./frames"
import { LogLabelBrowser } from "./label-browser"
import { LogsResults, type ContextRequest } from "./logs-results"
import { logsDatasourceTypes, logsTimeRange, type LogsSearch } from "./search"
import { traceDatasourceFor } from "./trace-link"
import { useElementSize } from "./use-element-size"
import { useLiveTail } from "./use-live-tail"

const routeId = "/_authed/_shell/logs"
const resultRows = 1000
const skeletonRows = [0, 1, 2, 3, 4]

function ignoreLint() {}

function useLogsSearch() {
  const search = useSearch({ from: routeId })
  const navigate = useNavigate({ from: "/logs" })
  const update = (next: Partial<LogsSearch>, replace = false) =>
    void navigate({ search: (state) => ({ ...state, ...next }), replace })
  return { search, update }
}

export function useDefaultDatasource(
  current: DatasourceStatus | undefined,
  eligible: readonly DatasourceStatus[]
) {
  const navigate = useNavigate()
  const fallback = current === undefined ? eligible[0]?.uid : undefined
  useEffect(() => {
    if (fallback === undefined) return
    void navigate({
      to: ".",
      search: (state: Record<string, unknown>) => ({ ...state, ds: fallback }),
      replace: true,
    })
  }, [fallback, navigate])
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
          No logs datasource is configured. An Admin adds a Loki or ClickHouse
          datasource in the datasources file.
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
    <LogsWorkspace
      key={datasource.uid}
      datasource={datasource}
      all={datasources.data ?? []}
      search={search}
      update={update}
    />
  )
}

function ResultSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {skeletonRows.map((row) => (
        <Skeleton key={row} className="h-5" />
      ))}
    </div>
  )
}

function LogsWorkspace({
  datasource,
  all,
  search,
  update,
}: {
  datasource: DatasourceStatus
  all: DatasourceStatus[]
  search: LogsSearch
  update: (next: Partial<LogsSearch>, replace?: boolean) => void
}) {
  const ids = useId()
  const uid = datasource.uid
  const type = datasource.type
  const range = logsTimeRange(search)
  const q = search.q
  const [draft, setDraft] = useState(
    () => q ?? (type === "clickhouse" ? clickhouseLogsTemplate : "")
  )
  const [draftProblem, setDraftProblem] = useState<string | null>(null)
  const [browserOpen, setBrowserOpen] = useState(false)
  const [tailSession, setTailSession] = useState(0)
  const [tailing, setTailing] = useState(false)
  const [tailShown, setTailShown] = useState(false)
  const [context, setContext] = useState<ContextRequest | null>(null)
  const [adding, setAdding] = useState(false)
  const [announcement, setAnnouncement] = useState({ text: "", count: 0 })
  const editorRef = useRef<CodeEditorHandle>(null)
  const contextTrigger = useRef<HTMLElement | null>(null)
  const addTrigger = useRef<HTMLButtonElement>(null)
  const { ref: resultsRef, height } = useElementSize<HTMLElement>()

  const results = useQuery({
    ...exploreQuery(uid, {
      query: q ?? "",
      kind: "instant",
      from: range.from,
      to: range.to,
      limit: resultRows,
    }),
    enabled: q !== undefined,
  })
  const indexLabels = useQuery({
    ...labelNamesQuery(uid, {}),
    enabled: type === "loki",
  })
  const tail = useLiveTail({
    uid,
    query: q ?? "",
    seed: results.data?.frames ?? [],
    running: tailing,
    session: tailSession,
  })

  const traceDatasource = traceDatasourceFor(datasource, all)
  const frames = tailShown ? tail.buffer.frames : results.data?.frames
  const lines = frames ? rowCount(frames) : 0
  const tailActive = tailing && tail.error === null

  function announce(text: string) {
    setAnnouncement((current) => ({ text, count: current.count + 1 }))
  }

  function stopTail() {
    if (!tailing) return
    setTailing(false)
    announce(`Live tail stopped, ${lines.toLocaleString("en")} lines`)
  }

  function startTail() {
    setTailSession((session) => session + 1)
    setTailing(true)
    setTailShown(true)
    announce("Live tail on")
  }

  function run() {
    const text = draft.trim()
    if (text === "") {
      setDraftProblem("Write a query first.")
      return
    }
    setDraftProblem(null)
    setTailing(false)
    setTailShown(false)
    if (text === q) void results.refetch()
    else update({ q: text })
  }

  function onEditorKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault()
      event.stopPropagation()
      run()
    }
  }

  function insert(text: string) {
    if (type === "loki" && draft.trim() === "") {
      setDraft(`{${text}}`)
      return
    }
    editorRef.current?.insertAtCursor(text)
  }

  function openContext(request: ContextRequest) {
    contextTrigger.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null
    setAdding(false)
    setContext(request)
  }

  function resultBody() {
    if (q === undefined) {
      return (
        <p className="text-[13px] text-muted-foreground">
          {type === "loki"
            ? 'Pick a label in the label browser or write a stream selector such as {service_name="checkout"}, then press Run query.'
            : "Edit the query and press Run query."}
        </p>
      )
    }
    if (!tailShown && results.fetchStatus === "paused" && !results.data) {
      return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
    }
    if (!tailShown && results.isError) {
      return (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {exploreErrorText(results.error)}
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => void results.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )
    }
    if (frames === undefined) return <ResultSkeleton />
    if (lines === 0) {
      return (
        <p className="text-[13px] text-muted-foreground">
          No lines in {lowerFirst(describeRange(range.from, range.to))}. Widen
          the time range or loosen the query.
        </p>
      )
    }
    return height > 0 ? (
      <LogsResults
        frames={frames}
        datasource={datasource}
        onContext={openContext}
        traceDatasource={traceDatasource}
        height={height}
      />
    ) : null
  }

  return (
    <>
      <PageHeader
        title="Logs"
        actions={
          <>
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
              aria-pressed={tailActive}
              disabled={q === undefined}
              onClick={() => (tailActive ? stopTail() : startTail())}
            >
              {tailActive ? "Stop live tail" : "Live tail"}
            </Button>
            <Button
              ref={addTrigger}
              type="button"
              variant="outline"
              disabled={q === undefined}
              onClick={() => {
                setContext(null)
                setAdding(true)
              }}
            >
              Add to dashboard
            </Button>
          </>
        }
      />
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-3">
          <ExploreDatasourcePicker
            id={`${ids}-datasource`}
            types={logsDatasourceTypes}
            value={uid}
            onChange={(next) => update({ ds: next, q: undefined })}
            label="Datasource"
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-expanded={browserOpen}
            aria-controls={`${ids}-labels`}
            onClick={() => setBrowserOpen((open) => !open)}
          >
            Label browser
          </Button>
        </div>
        {browserOpen ? (
          <div id={`${ids}-labels`}>
            <LogLabelBrowser
              datasourceUid={uid}
              datasourceType={type}
              onInsert={insert}
            />
          </div>
        ) : null}
        <div
          onKeyDownCapture={onEditorKeyDown}
          className="rounded-card border border-border"
        >
          <CodeEditor
            value={draft}
            onChange={setDraft}
            datasource={{ type, uid }}
            readOnly={false}
            ariaLabel="Log query"
            onLint={ignoreLint}
            editorRef={editorRef}
          />
        </div>
        {draftProblem ? (
          <p role="alert" className="text-xs text-destructive">
            {draftProblem}
          </p>
        ) : null}
        <div className="flex items-center gap-2">
          <Button
            type="button"
            title="Run query (Ctrl+Enter or Cmd+Enter)"
            onClick={run}
          >
            Run query
          </Button>
        </div>
        {tail.error ? (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-2">
              Live tail stopped: {lowerFirst(exploreErrorText(tail.error))}
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={startTail}
              >
                Start again
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}
        {lines === resultRows ? (
          <p className="text-[13px] text-muted-foreground">
            Showing the newest 1,000 lines. Narrow the query or the time range.
          </p>
        ) : null}
        <section
          ref={resultsRef}
          aria-label="Results"
          aria-busy={results.isFetching || tail.isFetching}
          className="relative h-[calc(100dvh-20rem)] min-h-96"
        >
          <div className="absolute inset-0 flex flex-col gap-2">
            {resultBody()}
          </div>
        </section>
        <span role="status" className="sr-only">
          {liveRegionText(announcement.text, announcement.count)}
        </span>
      </div>
      {context ? (
        <ContextPanel
          datasource={datasource}
          anchor={context}
          indexedLabels={indexLabels.data?.labels}
          onClose={() => setContext(null)}
          trigger={contextTrigger}
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
    </>
  )
}
