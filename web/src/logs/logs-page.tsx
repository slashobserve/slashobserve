import {
  useCallback,
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
} from "react"
import { useQuery } from "@tanstack/react-query"
import {
  Link,
  useNavigate,
  useRouteContext,
  useSearch,
} from "@tanstack/react-router"

import { AskField } from "@/ask/ask-field"
import { draftFailure, draftQueryRequest } from "@/ask/draft-query"
import { nextMode, useInputMode, type InputMode } from "@/ask/input-mode"
import { InputModeToggle } from "@/ask/input-mode-toggle"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { liveRegionText } from "@/lib/live-region"
import { roleAtLeast } from "@/lib/roles"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import { cn } from "@/lib/utils"
import { useAgentInfo } from "@/observe/queries"
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
import { streamLimit, streamLimitText } from "./stream-limit"
import { traceDatasourceFor } from "./trace-link"
import { useElementSize } from "./use-element-size"
import { useLiveTail } from "./use-live-tail"
import { VolumeStrip } from "./volume-strip"

const routeId = "/_authed/_shell/logs"
const resultRows = 1000
const skeletonRows = [0, 1, 2, 3, 4]
const askPlaceholder =
  "Ask in plain language, like: errors from checkout in the last hour with a trace id"

type AskProblem =
  { kind: "cannot_translate"; reason: string } | { kind: "error"; text: string }

type Answer = { query: string; explanation: string; warnings: string[] }

type FocusRequest = { target: InputMode | "labels"; count: number }

function ignoreLint() {}

function useLogsSearch() {
  const search = useSearch({ from: routeId })
  const navigate = useNavigate({ from: "/logs" })
  const update = useCallback(
    (next: Partial<LogsSearch>, replace = false) =>
      void navigate({ search: (state) => ({ ...state, ...next }), replace }),
    [navigate]
  )
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
  const [browserOpen, setBrowserOpen] = useState(
    type === "loki" && q === undefined
  )
  const [tailSession, setTailSession] = useState(0)
  const [tailing, setTailing] = useState(false)
  const [tailShown, setTailShown] = useState(false)
  const [context, setContext] = useState<ContextRequest | null>(null)
  const [adding, setAdding] = useState(false)
  const [announcement, setAnnouncement] = useState({ text: "", count: 0 })
  const editorRef = useRef<CodeEditorHandle>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const labelsRef = useRef<HTMLDivElement>(null)
  const askAbort = useRef<AbortController | null>(null)
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available === true
  const [mode, setMode] = useInputMode({
    agentAvailable,
    hasQuery: q !== undefined,
    hasAsk: search.ask !== undefined,
  })
  const [question, setQuestion] = useState(search.ask ?? "")
  const [fromQuestion, setFromQuestion] = useState<string | null>(() =>
    q !== undefined && search.ask !== undefined ? search.ask : null
  )
  const [answer, setAnswer] = useState<Answer | null>(null)
  const [asking, setAsking] = useState(false)
  const [askProblem, setAskProblem] = useState<AskProblem | null>(null)
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null)
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

  useEffect(() => {
    if (type === "clickhouse" && q === undefined) {
      update({ q: clickhouseLogsTemplate }, true)
    }
  }, [type, q, update])

  useEffect(() => () => askAbort.current?.abort(), [])

  useEffect(() => {
    if (focusRequest === null) return
    if (focusRequest.target === "ask") askFieldRef.current?.focus()
    else if (focusRequest.target === "labels") labelsRef.current?.focus()
    else editorRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
  }, [focusRequest])

  function focusNext(target: FocusRequest["target"]) {
    setFocusRequest((current) => ({
      target,
      count: (current?.count ?? 0) + 1,
    }))
  }

  function switchMode(next: InputMode) {
    if (next === mode) return
    setMode(next)
    focusNext(next)
  }

  useInputModeShortcut(() => switchMode(nextMode(mode, agentAvailable)))

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

  function runText(raw: string, asked?: string) {
    const text = raw.trim()
    if (text === "") {
      setDraftProblem("Write a query first.")
      return
    }
    setDraftProblem(null)
    setAskProblem(null)
    setTailing(false)
    setTailShown(false)
    if (text === q) void results.refetch()
    if (text !== q || (asked !== undefined && asked !== search.ask)) {
      update(asked === undefined ? { q: text } : { q: text, ask: asked })
    }
  }

  function run() {
    runText(draft)
  }

  async function ask(text: string) {
    askAbort.current?.abort()
    const controller = new AbortController()
    askAbort.current = controller
    setAsking(true)
    setAskProblem(null)
    try {
      const drafted = await draftQueryRequest(
        uid,
        { question: text, from: range.from, to: range.to, signal: "logs" },
        controller.signal
      )
      if (controller.signal.aborted) return
      setDraft(drafted.query)
      setAnswer({
        query: drafted.query,
        explanation: drafted.explanation,
        warnings: drafted.warnings,
      })
      setFromQuestion(text)
      runText(drafted.query, text)
    } catch (error) {
      const failure = draftFailure(error)
      if (failure.kind === "aborted") return
      setAskProblem(failure)
      if (failure.kind === "cannot_translate") {
        setMode("query", false)
        focusNext("query")
      }
    } finally {
      if (askAbort.current === controller) {
        askAbort.current = null
        setAsking(false)
      }
    }
  }

  function onDraftChange(next: string) {
    setDraft(next)
    if (mode === "ask" && answer !== null && next !== answer.query) {
      setMode("query", false)
    }
  }

  function askAgain() {
    if (fromQuestion !== null) setQuestion(fromQuestion)
    switchMode("ask")
  }

  function openLabelBrowser() {
    setBrowserOpen(true)
    focusNext("labels")
  }

  function onEditorKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
      event.preventDefault()
      event.stopPropagation()
      run()
    }
  }

  function insert(text: string) {
    if (mode === "ask") setMode("query", false)
    if (type === "loki" && draft.trim() === "") {
      setDraft(`{${text}}`)
      return
    }
    if (editorRef.current === null) setDraft(draft + text)
    else editorRef.current.insertAtCursor(text)
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
      return type === "loki" ? (
        <p className="text-[13px] text-muted-foreground">
          Pick a label to start, or write a stream selector such as
          {' {service_name="checkout"}.'}
        </p>
      ) : (
        <ResultSkeleton />
      )
    }
    if (!tailShown && results.fetchStatus === "paused" && !results.data) {
      return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
    }
    if (!tailShown && results.isError) {
      const limit = streamLimit(results.error)
      if (limit !== null) {
        return (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-2">
              {streamLimitText(limit)}
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={openLabelBrowser}
              >
                Open label browser
              </Button>
            </AlertDescription>
          </Alert>
        )
      }
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

  const notRunId = `${ids}-not-run`
  const editorShown =
    mode === "query" || answer !== null || fromQuestion !== null

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
              title={q === undefined ? "Run a query first." : undefined}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => (tailActive ? stopTail() : startTail())}
            >
              {tailActive ? "Stop live tail" : "Live tail"}
            </Button>
            <Button
              ref={addTrigger}
              type="button"
              variant="outline"
              disabled={q === undefined}
              title={q === undefined ? "Run a query first." : undefined}
              aria-describedby={q === undefined ? notRunId : undefined}
              onClick={() => {
                setContext(null)
                setAdding(true)
              }}
            >
              Add to dashboard
            </Button>
            {q === undefined ? (
              <span id={notRunId} className="sr-only">
                Run a query first.
              </span>
            ) : null}
          </>
        }
      />
      <div className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <InputModeToggle
            mode={mode}
            agentAvailable={agentAvailable}
            onChange={switchMode}
          />
          <div className="flex flex-wrap items-center gap-2">
            <ExploreDatasourcePicker
              id={`${ids}-datasource`}
              types={logsDatasourceTypes}
              value={uid}
              onChange={(next) =>
                update({ ds: next, q: undefined, ask: undefined })
              }
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
        </div>
        {browserOpen ? (
          <div
            id={`${ids}-labels`}
            ref={labelsRef}
            tabIndex={-1}
            aria-label="Label browser"
            role="region"
            className="outline-none"
          >
            <LogLabelBrowser
              datasourceUid={uid}
              datasourceType={type}
              onInsert={insert}
            />
          </div>
        ) : null}
        {mode === "ask" ? (
          <div className="[&_textarea]:pr-2.5">
            <AskField
              ref={askFieldRef}
              signal="logs"
              value={question}
              onChange={setQuestion}
              onAsk={(text) => void ask(text)}
              pending={asking}
              onCancel={() => askAbort.current?.abort()}
              placeholder={askPlaceholder}
            />
          </div>
        ) : fromQuestion !== null ? (
          <p className="text-xs text-muted-foreground">
            From: {fromQuestion}{" "}
            <button
              type="button"
              onClick={askAgain}
              className="text-foreground underline underline-offset-3"
            >
              Ask again
            </button>
          </p>
        ) : null}
        {mode === "ask" && answer !== null ? (
          <div className="flex flex-col gap-1">
            {answer.explanation !== "" ? (
              <p className="text-xs text-muted-foreground">
                {answer.explanation}
              </p>
            ) : null}
            {answer.warnings.map((warning) => (
              <p key={warning} className="text-xs">
                <StatusLabel tone="warning">{warning}</StatusLabel>
              </p>
            ))}
          </div>
        ) : null}
        {askProblem !== null ? (
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-1">
              {askProblem.kind === "cannot_translate" ? (
                <>
                  <p>I could not turn that into a query.</p>
                  <p className="text-muted-foreground">{askProblem.reason}</p>
                </>
              ) : (
                <p>{askProblem.text}</p>
              )}
            </AlertDescription>
          </Alert>
        ) : null}
        {editorShown ? (
          <div
            onKeyDownCapture={onEditorKeyDown}
            className={cn(mode === "ask" && "[&_.cm-editor]:max-h-44!")}
          >
            <CodeEditor
              value={draft}
              onChange={onDraftChange}
              datasource={{ type, uid }}
              readOnly={false}
              ariaLabel="Log query"
              onLint={ignoreLint}
              editorRef={editorRef}
            />
          </div>
        ) : null}
        {draftProblem ? (
          <p role="alert" className="text-xs text-destructive">
            {draftProblem}
          </p>
        ) : null}
        {editorShown ? (
          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              variant={mode === "ask" ? "outline" : "default"}
              title="Run query (Ctrl+Enter or Cmd+Enter)"
              aria-keyshortcuts="Control+Enter Meta+Enter"
              onClick={run}
            >
              Run query
            </Button>
            <span className="text-xs text-muted-foreground">
              or press Ctrl+Enter or Cmd+Enter
            </span>
          </div>
        ) : null}
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
        {q !== undefined && !tailShown ? (
          <VolumeStrip uid={uid} query={q} from={range.from} to={range.to} />
        ) : null}
        <section
          ref={resultsRef}
          aria-label="Results"
          aria-busy={results.isFetching || tail.isFetching}
          className="relative h-[calc(100dvh-23rem)] min-h-96"
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
