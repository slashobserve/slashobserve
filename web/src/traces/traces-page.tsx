import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { useNavigate, useSearch } from "@tanstack/react-router"
import { CircleAlertIcon } from "lucide-react"

import { AskField } from "@/ask/ask-field"
import { nextMode, useInputMode, type InputMode } from "@/ask/input-mode"
import { InputModeToggle } from "@/ask/input-mode-toggle"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { PageBar } from "@/components/page-bar"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { PanelDataResponse } from "@/dashboards/queries"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import {
  rememberDatasource,
  rememberedDatasource,
} from "@/datasources/datasource-memory"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import {
  defaultPageSize,
  pagesByOffset,
  pageStateOf,
  type PageSize,
} from "@/lib/paging"
import { isModEnter } from "@/lib/shortcuts"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import { usePageCursors } from "@/lib/use-page-cursors"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "@/logs/datasource-picker"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { useAgentInfo } from "@/observe/queries"

import { traceSearchFromAnswer } from "./ask-answer"
import { ComparePanel } from "./compare-panel"
import type { Selection } from "./scatter"
import {
  traceDatasourceTypes,
  tracesTimeRange,
  type TraceSearch,
} from "./search"
import {
  attributeFilter,
  keepSelectionSearch,
  traceSearchParams,
  traceSearchParamsWithCursor,
  traceStatsFilter,
} from "./search-query"
import { TraceSearchForm } from "./trace-search-form"
import { countText, durationText, traceRows, type TraceRow } from "./trace-rows"
import {
  histogramPercentile,
  p95Label,
  partialNote,
  traceStatsQuery,
} from "./trace-stats"
import { TraceTable } from "./trace-table"

const skeletonRows = [0, 1, 2, 3, 4]
const askPlaceholder =
  "Ask in plain language, like: checkout requests slower than 2s with errors"

const tempoPagingNote =
  "Tempo pages by start time from the last trace shown, so there is no page count."

type FocusRequest = { target: InputMode; count: number }

type UpdateSearch = (
  next: Partial<TraceSearch>,
  options?: { replace?: boolean }
) => void

function useTraceSearch() {
  const search = useSearch({ from: "/_authed/_shell/traces" })
  const navigate = useNavigate({ from: "/traces" })
  const update: UpdateSearch = (next, options) =>
    void navigate({
      search: (state) => ({ ...state, page: undefined, ...next }),
      replace: options?.replace,
    })
  return { search, update }
}

function useTraceDatasource(
  current: DatasourceStatus | undefined,
  eligible: readonly DatasourceStatus[]
) {
  const navigate = useNavigate()
  const fallback =
    current === undefined ? rememberedDatasource("traces", eligible) : undefined
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
    if (uid !== undefined) rememberDatasource("traces", uid)
  }, [uid])
}

export function TracesPage() {
  const ids = useId()
  const { search, update } = useTraceSearch()
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, traceDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  useTraceDatasource(datasource, eligible)
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
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title="Traces"
        actions={
          <>
            <ExploreDatasourcePicker
              id={`${ids}-datasource`}
              types={traceDatasourceTypes}
              value={datasource?.uid}
              onChange={(ds) => update({ ds, ask: undefined })}
              label="Datasource"
            />
            <TimeRangeControl value={range} onChange={update} />
          </>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col gap-3">
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
    </div>
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
  update: UpdateSearch
}) {
  const formId = useId()
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
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const serviceRef = useRef<HTMLInputElement>(null)
  const ask = useAsk({
    uid,
    signal: "traces",
    range,
    initialQuestion: search.ask,
    initialFromQuestion: search.ask ?? null,
    onAnswer: (drafted, question) =>
      update({ ...traceSearchFromAnswer(drafted, type), ask: question }),
    onCannotTranslate: () => {
      setMode("query", false)
      focusNext("query")
    },
  })

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

  function askAgain() {
    ask.askAgain()
    switchMode("ask")
  }

  function submitOnModEnter(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.currentTarget.querySelector("form")?.requestSubmit()
  }

  const answered = ask.answer !== null || ask.fromQuestion !== null

  return (
    <>
      <div className="flex h-8 shrink-0 items-center gap-2">
        <InputModeToggle
          mode={mode}
          agentAvailable={agentAvailable}
          onChange={switchMode}
        />
      </div>
      {mode === "ask" ? (
        <div className="w-full shrink-0 [&_textarea]:pr-2.5">
          <AskField
            ref={askFieldRef}
            signal="traces"
            value={ask.question}
            onChange={ask.setQuestion}
            onAsk={ask.ask}
            pending={ask.asking}
            onCancel={ask.cancel}
            placeholder={askPlaceholder}
          />
        </div>
      ) : (
        <>
          <div className="shrink-0" onKeyDownCapture={submitOnModEnter}>
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
              id={formId}
              type={type}
              uid={uid}
              value={search}
              onSubmit={(next) => {
                ask.clearProblem()
                update(next)
              }}
              serviceRef={serviceRef}
            />
          </div>
          <div className="flex h-8 shrink-0 items-center gap-3">
            <Button
              type="submit"
              form={formId}
              title="Search traces (Ctrl+Enter or Cmd+Enter)"
              aria-keyshortcuts="Control+Enter Meta+Enter"
            >
              Search traces
            </Button>
            <span className="text-xs text-muted-foreground">
              or press Ctrl+Enter or Cmd+Enter
            </span>
            {ask.fromQuestion !== null ? (
              <span className="min-w-0 truncate text-xs text-muted-foreground">
                From: {ask.fromQuestion}
              </span>
            ) : null}
            {answered || ask.problem !== null ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={askAgain}
              >
                Ask again
              </Button>
            ) : null}
          </div>
        </>
      )}
      {mode === "ask" && answered ? (
        <div className="flex shrink-0 flex-col items-start gap-1">
          {ask.answer !== null && ask.answer.explanation !== "" ? (
            <p className="text-xs text-muted-foreground">
              {ask.answer.explanation}
            </p>
          ) : null}
          {ask.answer?.warnings.map((warning) => (
            <p key={warning} className="text-xs">
              <StatusLabel tone="warning">{warning}</StatusLabel>
            </p>
          ))}
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => switchMode("query")}
          >
            Edit the query
          </Button>
        </div>
      ) : null}
      {ask.problem !== null ? (
        <Alert variant="destructive" className="shrink-0">
          <AlertDescription className="flex flex-col items-start gap-1">
            {ask.problem.kind === "cannot_translate" ? (
              <>
                <p>I could not turn that into a query.</p>
                <p className="text-muted-foreground">{ask.problem.reason}</p>
              </>
            ) : (
              <p>{ask.problem.text}</p>
            )}
          </AlertDescription>
        </Alert>
      ) : null}
      <div className="min-h-0 flex-1 overflow-auto">
        <TraceResults datasource={datasource} search={search} update={update} />
      </div>
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

type LoadedPage = { scope: string; data: PanelDataResponse }

function TraceResults({
  datasource,
  search,
  update,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: UpdateSearch
}) {
  const range = tracesTimeRange(search)
  const byOffset = pagesByOffset(datasource.type)
  const pageSize: PageSize = search.size ?? defaultPageSize
  const base = traceSearchParams(datasource.type, {
    ...search,
    page: undefined,
  })
  const scope = JSON.stringify([
    datasource.uid,
    base.query,
    base.from,
    base.to,
    pageSize,
  ])
  const cursors = usePageCursors(scope, byOffset)
  const requested = search.page ?? 1
  const page = cursors.reachable(requested) ? requested : 1
  const params = traceSearchParamsWithCursor(
    { ...base, page },
    byOffset ? undefined : cursors.cursorFor(page)
  )
  const results = useQuery(exploreQuery(datasource.uid, params))
  const stats = useQuery(
    traceStatsQuery(datasource.uid, {
      query: traceStatsFilter(datasource.type, search),
      ...range,
    })
  )
  const [lastGood, setLastGood] = useState<LoadedPage | null>(null)
  const [lost, setLost] = useState<{ scope: string; page: number } | null>(null)
  const fresh =
    results.data !== undefined && !results.isPlaceholderData
      ? results.data
      : undefined
  if (fresh !== undefined && lastGood?.data !== fresh) {
    setLastGood({ scope, data: fresh })
  }
  if (page !== requested && lost?.scope !== scope) {
    setLost({ scope, page: requested })
  }
  const nextCursor = fresh?.meta.page?.next
  const freshPage = fresh?.meta.page?.page
  const { remember } = cursors

  useEffect(() => {
    if (page !== requested) update({ page: undefined }, { replace: true })
  }, [page, requested, update])

  useEffect(() => {
    if (freshPage !== undefined) remember(freshPage, nextCursor)
  }, [freshPage, nextCursor, remember])

  const data =
    results.data ??
    (results.isError && lastGood?.scope === scope ? lastGood.data : undefined)

  if (results.fetchStatus === "paused" && !data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  const failure = results.isError ? (
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
  ) : null
  if (!data) return failure ?? <ResultSkeleton />
  const rows = traceRows(data.frames)
  const meta = data.meta
  const state = pageStateOf(meta.page, rows.length, byOffset, {
    page,
    pageSize,
    truncated: meta.truncated,
  })
  const pageBar = (
    <PageBar
      state={state}
      noun="traces"
      qualifier=" in range"
      note={datasource.type === "tempo" ? tempoPagingNote : undefined}
      label="Trace pages"
      busy={results.isFetching}
      onPage={(next) => {
        setLost(null)
        update({ page: next })
      }}
      onPageSize={(size) =>
        update({ size: size === defaultPageSize ? undefined : size })
      }
    />
  )
  if (rows.length === 0) {
    return (
      <div className="flex flex-col gap-2">
        {failure}
        <p className="text-[13px] text-muted-foreground">
          No traces match in {lowerFirst(describeRange(range.from, range.to))}.
        </p>
        {state.hasPrev ? pageBar : null}
      </div>
    )
  }
  const p95 =
    stats.data === undefined || stats.isError
      ? undefined
      : histogramPercentile(stats.data.histogram, 95)
  const label = p95Label(search)
  const covered = meta.partial ? partialNote(rows, undefined) : null
  return (
    <div className="flex flex-col gap-2">
      {failure}
      {lost !== null && lost.scope === scope ? (
        <p role="status" className="text-[13px] text-muted-foreground">
          Page {lost.page} could not be restored; showing page 1
        </p>
      ) : null}
      <p className="flex items-center gap-1.5 text-[13px] text-muted-foreground">
        <span>{countText(meta.rows, "trace")}</span>
        {stats.isError ? (
          <Tooltip>
            <TooltipTrigger
              render={<span className="inline-flex" />}
              aria-label="Stats failed"
            >
              <CircleAlertIcon
                aria-hidden="true"
                className="size-3.5 text-status-critical"
              />
            </TooltipTrigger>
            <TooltipContent>
              Stats failed: {exploreErrorText(stats.error)}
            </TooltipContent>
          </Tooltip>
        ) : null}
      </p>
      {meta.partial ? (
        <Alert>
          <AlertDescription className="flex flex-col items-start gap-2">
            <p>
              Tempo stopped searching at its time limit, so older traces may be
              missing.
              {covered !== null ? ` ${covered.text}` : null}
            </p>
            {covered !== null ? (
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={() => update({ from: covered.from, to: covered.to })}
              >
                Keep only {covered.covers}
              </Button>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}
      <SelectableTraceTable
        key={`${scope}:${state.page}`}
        datasource={datasource}
        search={search}
        update={update}
        rows={rows}
        p95={p95}
        label={label}
      />
      {pageBar}
    </div>
  )
}

const keepSelectionLabel = "Keep only the selection"

function KeepSelectionButton({
  next,
  onKeep,
}: {
  next: Partial<TraceSearch> | null
  onKeep: (next: Partial<TraceSearch>) => void
}) {
  if (next === null) {
    return (
      <Tooltip>
        <TooltipTrigger render={<span className="inline-flex" />}>
          <Button type="button" variant="outline" size="xs" disabled>
            {keepSelectionLabel}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          Cannot add duration bounds to this query.
        </TooltipContent>
      </Tooltip>
    )
  }
  return (
    <Button
      type="button"
      variant="outline"
      size="xs"
      onClick={() => onKeep(next)}
    >
      {keepSelectionLabel}
    </Button>
  )
}

function SelectableTraceTable({
  datasource,
  search,
  update,
  rows,
  p95,
  label,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: UpdateSearch
  rows: TraceRow[]
  p95: number | null | undefined
  label: string
}) {
  const range = tracesTimeRange(search)
  const [selection, setSelection] = useState<Selection | null>(null)

  function keep(next: Partial<TraceSearch>) {
    setSelection(null)
    update(next)
  }

  return (
    <TraceTable
      rows={rows}
      datasourceUid={datasource.uid}
      range={range}
      p95Ms={p95}
      p95Label={label}
      footerNote={
        p95 === undefined || p95 === null
          ? undefined
          : `The tick on each bar is the ${durationText(p95)} ${label} in this range.`
      }
      selection={selection}
      onSelectionChange={setSelection}
      selectionActions={
        selection === null ? undefined : (
          <KeepSelectionButton
            next={keepSelectionSearch(search, selection)}
            onKeep={keep}
          />
        )
      }
      selectionPanel={
        selection === null ? undefined : (
          <ComparePanel
            datasource={datasource}
            rows={rows}
            selection={selection}
            range={range}
            filterFor={(difference) =>
              attributeFilter(
                datasource.type,
                search,
                difference.attribute,
                difference.value
              )
            }
            onFilter={keep}
          />
        )
      }
    />
  )
}
