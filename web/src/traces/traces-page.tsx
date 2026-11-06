import { useEffect, useId, useRef, useState } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate, useSearch } from "@tanstack/react-router"

import { AskField } from "@/ask/ask-field"
import {
  draftFailure,
  draftQueryRequest,
  type DraftFailure,
} from "@/ask/draft-query"
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
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import { cn } from "@/lib/utils"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "@/logs/datasource-picker"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { useDefaultDatasource } from "@/logs/logs-page"
import { useAgentInfo } from "@/observe/queries"

import { traceSearchFromAnswer } from "./ask-answer"
import {
  traceDatasourceTypes,
  tracesTimeRange,
  type TraceSearch,
} from "./search"
import { traceSearchParams, traceSearchRows } from "./search-query"
import { TraceSearchForm } from "./trace-search-form"
import { traceRows } from "./trace-rows"
import { TraceTable } from "./trace-table"

const skeletonRows = [0, 1, 2, 3, 4]
const askPlaceholder =
  "Ask in plain language, like: checkout requests slower than 2s with errors"

type AskProblem = Exclude<DraftFailure, { kind: "aborted" }>

type Answer = { explanation: string; warnings: string[] }

type FocusRequest = { target: InputMode; count: number }

function useTraceSearch() {
  const search = useSearch({ from: "/_authed/_shell/traces" })
  const navigate = useNavigate({ from: "/traces" })
  const update = (next: Partial<TraceSearch>) =>
    void navigate({ search: (state) => ({ ...state, ...next }) })
  return { search, update }
}

export function TracesPage() {
  const ids = useId()
  const { search, update } = useTraceSearch()
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, traceDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  useDefaultDatasource(datasource, eligible)
  const range = tracesTimeRange(search)

  if (!datasources.isPending && eligible.length === 0) {
    return (
      <>
        <PageHeader title="Traces" />
        <p className="text-[13px] text-muted-foreground">
          No traces datasource is configured. An Admin adds a Tempo or
          ClickHouse datasource in the datasources file.
        </p>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Traces"
        actions={<TimeRangeControl value={range} onChange={update} />}
      />
      <div className="flex flex-col gap-4">
        <ExploreDatasourcePicker
          id={`${ids}-datasource`}
          types={traceDatasourceTypes}
          value={datasource?.uid}
          onChange={(ds) => update({ ds, ask: undefined })}
          label="Datasource"
        />
        {datasource ? (
          <TraceSearchWorkspace
            key={datasource.uid}
            datasource={datasource}
            search={search}
            update={update}
          />
        ) : (
          <ResultSkeleton />
        )}
      </div>
    </>
  )
}

function hasTraceFilters(search: TraceSearch): boolean {
  return [
    search.service,
    search.span,
    search.min,
    search.max,
    search.errors,
    search.q,
  ].some((value) => value !== undefined)
}

function TraceSearchWorkspace({
  datasource,
  search,
  update,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: (next: Partial<TraceSearch>) => void
}) {
  const uid = datasource.uid
  const type = datasource.type
  const range = tracesTimeRange(search)
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available === true
  const [mode, setMode] = useInputMode({
    agentAvailable,
    hasQuery: hasTraceFilters(search),
    hasAsk: search.ask !== undefined,
  })
  const [question, setQuestion] = useState(search.ask ?? "")
  const [fromQuestion, setFromQuestion] = useState<string | null>(
    search.ask ?? null
  )
  const [answer, setAnswer] = useState<Answer | null>(null)
  const [asking, setAsking] = useState(false)
  const [askProblem, setAskProblem] = useState<AskProblem | null>(null)
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const serviceRef = useRef<HTMLInputElement>(null)
  const askAbort = useRef<AbortController | null>(null)

  useEffect(() => () => askAbort.current?.abort(), [])

  useEffect(() => {
    if (focusRequest === null) return
    if (focusRequest.target === "ask") askFieldRef.current?.focus()
    else serviceRef.current?.focus()
  }, [focusRequest])

  function focusNext(target: InputMode) {
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

  async function ask(text: string) {
    askAbort.current?.abort()
    const controller = new AbortController()
    askAbort.current = controller
    setAsking(true)
    setAskProblem(null)
    try {
      const drafted = await draftQueryRequest(
        uid,
        { question: text, from: range.from, to: range.to, signal: "traces" },
        controller.signal
      )
      if (controller.signal.aborted) return
      setAnswer({
        explanation: drafted.explanation,
        warnings: drafted.warnings,
      })
      setFromQuestion(text)
      update({ ...traceSearchFromAnswer(drafted, type), ask: text })
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

  function askAgain() {
    if (fromQuestion !== null) setQuestion(fromQuestion)
    switchMode("ask")
  }

  const formShown = mode === "query" || fromQuestion !== null
  const modeToggle = (
    <div className="absolute top-2 right-2 z-10">
      <InputModeToggle
        mode={mode}
        agentAvailable={agentAvailable}
        onChange={switchMode}
      />
    </div>
  )

  return (
    <>
      {mode === "ask" ? (
        <div className="relative">
          {modeToggle}
          <AskField
            ref={askFieldRef}
            signal="traces"
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
      {formShown ? (
        <div
          className={cn(
            "relative rounded-card border border-border p-3",
            mode === "query" && "pt-10"
          )}
        >
          {mode === "query" ? modeToggle : null}
          <TraceSearchForm
            key={JSON.stringify([
              search.service,
              search.span,
              search.min,
              search.max,
              search.errors,
              search.q,
              search.ask,
            ])}
            type={type}
            uid={uid}
            value={search}
            onSubmit={update}
            answered={mode === "ask" && fromQuestion !== null}
            onEdited={() => setMode("query", false)}
            serviceRef={serviceRef}
          />
        </div>
      ) : null}
      <TraceResults datasource={datasource} search={search} />
    </>
  )
}

function ResultSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {skeletonRows.map((row) => (
        <Skeleton key={row} className="h-8" />
      ))}
    </div>
  )
}

function TraceResults({
  datasource,
  search,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
}) {
  const range = tracesTimeRange(search)
  const results = useQuery(
    exploreQuery(datasource.uid, traceSearchParams(datasource.type, search))
  )

  if (results.fetchStatus === "paused" && !results.data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  if (results.isError) {
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
  if (!results.data) return <ResultSkeleton />
  const rows = traceRows(results.data.frames)
  if (rows.length === 0) {
    return (
      <p className="text-[13px] text-muted-foreground">
        No traces match in {lowerFirst(describeRange(range.from, range.to))}.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-2">
      {rows.length >= traceSearchRows ? (
        <p className="text-[13px] text-muted-foreground">
          Showing the newest 100 traces. Narrow the search.
        </p>
      ) : null}
      <TraceTable rows={rows} datasourceUid={datasource.uid} range={range} />
    </div>
  )
}
