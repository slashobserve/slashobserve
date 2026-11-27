import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"

import { AskedFrom, AskProblem } from "@/ask/ask-result"
import { loadProblemText } from "@/api/errors"
import { AskField } from "@/ask/ask-field"
import type { InputMode } from "@/ask/input-mode"
import { InputModeToggle } from "@/ask/input-mode-toggle"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { AgentMark } from "@/components/agent-mark"
import { PageBar } from "@/components/page-bar"
import { LoadProblem } from "@/components/load-problem"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { PanelDataResponse } from "@/dashboards/queries"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { SourceTitle } from "@/datasources/source-title"
import { UnknownSource } from "@/datasources/unknown-source"
import { useExploreSources } from "@/datasources/use-sources"
import { foldersQuery } from "@/folders/queries"
import { useDocumentTitle } from "@/lib/document-title"
import {
  defaultPageSize,
  pagesByOffset,
  pageStateOf,
  type PageSize,
} from "@/lib/paging"
import { isModEnter } from "@/lib/shortcuts"
import { tracesLink } from "@/lib/links"
import { lowerFirst } from "@/lib/text"
import { describeRange, exploreRange } from "@/lib/time"
import { useAfterMainQuery } from "@/lib/use-after-main-query"
import { usePageCursors } from "@/lib/use-page-cursors"
import { cn } from "@/lib/utils"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { ExploreFailure } from "@/logs/explore-failure"
import { useAgentInfo } from "@/observe/queries"
import type { CodeEditorHandle } from "@/query-editor/code-editor"
import { saveBlockedText, tracesDraft } from "@/saved-searches/draft"
import { OpenSavedSearch } from "@/saved-searches/open-saved-search"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { ExploreActions } from "@/saved-searches/explore-actions"
import { useLeaveAskOnSavedOpen } from "@/saved-searches/saved-open"

import { AddToDashboardAction } from "./add-to-dashboard-action"
import { AddToObservationAction } from "./add-to-observation-action"
import { traceSearchFromAnswer } from "./ask-answer"
import { ComparePanel } from "./compare-panel"
import type { Selection } from "./scatter"
import {
  askAfterSubmit,
  changesQuery,
  hasTraceStats,
  linkWithoutPage,
  rangeInFuture,
  type TraceSearch,
} from "./search"
import {
  attributeFilter,
  keepSelectionSearch,
  traceSearchParams,
  traceSearchParamsWithCursor,
  traceStatsFilter,
} from "./search-query"
import { isRelativeRange, liveBlockedText, liveRefreshMs } from "./live-refresh"
import { traceFilterChips } from "./trace-filters"
import { tracePanelQuery, tracePanelTitle, tracesPanelFor } from "./trace-panel"
import { shorterRange, traceQLProblem } from "./trace-query-problem"
import { TraceSearchForm } from "./trace-search-form"
import { durationText, traceRows, type TraceRow } from "./trace-rows"
import { useTraceInputMode } from "./trace-input-mode"
import { traceSqlSearch, traceSqlStart } from "./trace-sql"
import { TraceSqlEditor, TraceSqlResults } from "./trace-sql-view"
import {
  histogramPercentile,
  p95Label,
  noTracesText,
  partialNote,
  partialReason,
  traceStatsQuery,
} from "./trace-stats"
import { decodeSort, encodeSort, listOrder } from "./trace-rows"
import { TraceTable, type TraceSortControl } from "./trace-table"

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
  const search = useSearch({ from: "/_authed/_shell/traces/$datasourceUid" })
  const navigate = useNavigate({ from: "/traces/$datasourceUid" })
  const update: UpdateSearch = (next, options) =>
    void navigate({
      search: (state) => ({ ...state, page: undefined, ...next }),
      replace: options?.replace,
    })
  return { search, update }
}

export function TracesPage({ uid }: { uid: string }) {
  const { search, update } = useTraceSearch()
  const { sources, isPending, isError, error, refetch } =
    useExploreSources("traces")
  const datasource = sources.find((candidate) => candidate.uid === uid)
  const rememberedUid = datasource?.uid
  useDocumentTitle(
    datasource
      ? `Traces / ${datasource.name} - SlashObserve`
      : "Traces - SlashObserve"
  )
  useEffect(() => {
    if (rememberedUid !== undefined) {
      useSourceMemory.getState().remember("traces", rememberedUid)
    }
  }, [rememberedUid])
  const range = exploreRange(search)
  const folders = useQuery(foldersQuery())
  const saveBlocked = saveBlockedText(folders.data)
  const [saving, setSaving] = useState(false)
  const [live, setLive] = useState(false)
  const saveTrigger = useRef<HTMLButtonElement>(null)
  if (live && !isRelativeRange(range)) setLive(false)

  if (isPending) {
    return (
      <>
        <h1 className="sr-only">Traces</h1>
        <ResultSkeleton />
      </>
    )
  }
  if (isError) {
    return (
      <>
        <h1 className="sr-only">Traces</h1>
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
        <h1 className="sr-only">Traces</h1>
        <p className="text-[13px] text-muted-foreground">
          No traces datasource is configured. An Admin adds a Tempo, Jaeger or
          ClickHouse datasource in the datasources file.
        </p>
      </>
    )
  }
  if (datasource === undefined) {
    return (
      <UnknownSource kind="traces" uid={uid}>
        <ul className="mb-4 flex flex-col gap-1 text-[13px]">
          {sources.map((source) => (
            <li key={source.uid}>
              <Link
                {...tracesLink(source.uid)}
                className="underline underline-offset-3"
              >
                Open {source.name}
              </Link>
            </li>
          ))}
        </ul>
      </UnknownSource>
    )
  }

  const draft = tracesDraft({
    uid: datasource.uid,
    search,
    range,
  })

  return (
    <div className="flex flex-col gap-3">
      <h1 className="sr-only">Traces in {datasource.name}</h1>
      <OpenSavedSearch
        kind="traces"
        uid={datasource.uid}
        savedId={search.saved}
        draft={draft}
        onClose={() => update({ saved: undefined }, { replace: true })}
      />
      <ExploreActions
        kind="traces"
        range={range}
        onRangeChange={update}
        tail={{
          on: live,
          blocked: isRelativeRange(range) ? undefined : liveBlockedText,
          toggle: () => setLive(!live),
        }}
        saveTrigger={saveTrigger}
        saveBlocked={saveBlocked}
        shareHref={
          datasource.type === "tempo"
            ? linkWithoutPage(window.location.href)
            : window.location.href
        }
        onSave={() => setSaving(true)}
      />
      <SaveSearchDialog
        draft={draft}
        open={saving}
        onOpenChange={setSaving}
        finalFocus={() => saveTrigger.current ?? true}
      />
      <TraceSearchWorkspace
        key={datasource.uid}
        datasource={datasource}
        search={search}
        update={update}
        live={live}
      />
    </div>
  )
}

function TraceSearchWorkspace({
  datasource,
  search,
  update,
  live,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: UpdateSearch
  live: boolean
}) {
  const formId = useId()
  const uid = datasource.uid
  const type = datasource.type
  const clickhouse = type === "clickhouse"
  const range = exploreRange(search)
  const agent = useAgentInfo()
  const agentAvailable = agent.data?.available === true
  const inputMode = useTraceInputMode({ type, search, agentAvailable })
  const { shown, formMode } = inputMode
  const [focusRequest, setFocusRequest] = useState<FocusRequest | null>(null)
  const askFieldRef = useRef<HTMLTextAreaElement>(null)
  const serviceRef = useRef<HTMLInputElement>(null)
  const sqlRef = useRef<CodeEditorHandle>(null)
  const ask = useAsk({
    uid,
    signal: "traces",
    range,
    initialQuestion: search.ask,
    initialFromQuestion: search.ask ?? null,
    onAnswer: (drafted, question) =>
      update({ ...traceSearchFromAnswer(drafted, type), ask: question }),
    onCannotTranslate: () => {
      inputMode.select(formMode, false)
      focusNext(formMode)
    },
  })

  useEffect(() => {
    if (focusRequest === null) return
    if (focusRequest.target === "ask") askFieldRef.current?.focus()
    else if (focusRequest.target === "code" && clickhouse)
      sqlRef.current?.focusAt(Number.MAX_SAFE_INTEGER)
    else serviceRef.current?.focus()
  }, [focusRequest, clickhouse])

  function focusNext(target: InputMode) {
    setFocusRequest((current) => ({
      target,
      count: (current?.count ?? 0) + 1,
    }))
  }

  function switchMode(next: InputMode) {
    if (next === shown) return
    inputMode.select(next)
    focusNext(next)
  }

  useInputModeShortcut(() => switchMode(inputMode.following()))
  useLeaveAskOnSavedOpen(shown, (_, remember) =>
    inputMode.select(
      clickhouse && search.q === undefined ? "builder" : "code",
      remember
    )
  )

  function askAgain() {
    ask.askAgain()
    switchMode("ask")
  }

  function submitOnModEnter(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.currentTarget.querySelector("form")?.requestSubmit()
  }

  const answered = search.ask !== undefined

  const chips = traceFilterChips(type, search)
  const explanation = ask.answer?.explanation ?? ""
  const askRow = shown === "ask"
  const sqlRow = inputMode.sqlShown

  return (
    <>
      <SourceTitle
        kind="traces"
        source={datasource}
        language={inputMode.language}
      />
      <section
        aria-label="Query"
        className="flex flex-col rounded-card border bg-card"
      >
        <div
          className={cn(
            "flex items-start gap-2 px-4 pt-3 narrow:flex-col narrow:items-stretch",
            askRow && !answered && "pb-3"
          )}
        >
          <InputModeToggle
            mode={shown}
            modes={inputMode.modes}
            labels={inputMode.labels}
            agentAvailable={agentAvailable}
            onChange={switchMode}
          />
          {askRow ? (
            <div className="min-w-0 flex-1">
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
          ) : null}
        </div>
        {askRow && answered ? (
          <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-1 pt-2 pr-3 pb-2.5 pl-4">
            <div className="flex min-w-0 flex-1 flex-col gap-1.5">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2.5 gap-y-1.5">
                {explanation !== "" ? (
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    <AgentMark size={20} className="text-primary" />
                    <span className="min-w-0">{explanation}</span>
                  </p>
                ) : null}
                {chips.length > 0 ? (
                  <div
                    role="group"
                    aria-label="The query Ask wrote"
                    className="flex flex-wrap gap-1.5"
                  >
                    {chips.map((chip) => (
                      <span
                        key={chip.key}
                        className="inline-flex h-[22px] items-center gap-1.5 rounded-chip border bg-muted/50 px-1.5 text-xs whitespace-nowrap"
                      >
                        <span className="text-muted-foreground">
                          {chip.label}
                        </span>
                        {chip.value !== "" ? (
                          <span className="font-mono">{chip.value}</span>
                        ) : null}
                      </span>
                    ))}
                  </div>
                ) : null}
              </div>
              {ask.answer?.warnings.map((warning) => (
                <p key={warning} className="text-xs">
                  <StatusLabel tone="warning">{warning}</StatusLabel>
                </p>
              ))}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="narrow:h-11"
              onClick={() => switchMode(formMode)}
            >
              Edit the query
            </Button>
          </div>
        ) : null}
        {sqlRow ? (
          <TraceSqlEditor
            key={search.q ?? ""}
            datasource={datasource}
            initial={traceSqlStart(search)}
            editorRef={sqlRef}
            onRun={(query) => {
              ask.clearProblem()
              update(traceSqlSearch(query))
            }}
          />
        ) : null}
        {askRow || sqlRow ? null : (
          <>
            <div className="px-4 pt-3 pb-4" onKeyDownCapture={submitOnModEnter}>
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
                  update({ ...next, ask: askAfterSubmit(search, next) })
                }}
                serviceRef={serviceRef}
              />
            </div>
            <div className="flex flex-wrap items-center gap-3 border-t px-4 py-2.5">
              <Button
                type="submit"
                form={formId}
                size="sm"
                className="narrow:h-11"
                title="Search traces (Ctrl+Enter or Cmd+Enter)"
                aria-keyshortcuts="Control+Enter Meta+Enter"
              >
                Search traces
              </Button>
              <span className="text-xs text-muted-foreground">
                or press Ctrl+Enter or Cmd+Enter
              </span>
              <AskedFrom
                question={search.ask}
                onWriteAgain={
                  answered || ask.problem !== null ? askAgain : undefined
                }
              />
            </div>
          </>
        )}
        {ask.problem !== null ? (
          <div className="border-t px-4 py-2.5">
            <AskProblem problem={ask.problem} />
          </div>
        ) : null}
      </section>
      {clickhouse && search.q !== undefined ? (
        <TraceSqlResults
          datasource={datasource}
          query={search.q}
          search={search}
          onPage={(page) => update({ page })}
          onPageSize={(size) => update({ size })}
          live={live}
        />
      ) : (
        <TraceResults
          datasource={datasource}
          search={search}
          update={(next, options) => {
            if (changesQuery(next)) {
              inputMode.select(formMode, false)
              update({ ask: undefined, ...next }, options)
            } else update(next, options)
          }}
          live={live}
        />
      )}
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
  live,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: UpdateSearch
  live: boolean
}) {
  const range = exploreRange(search)
  const [openedAt] = useState(Date.now)
  const polling = {
    refetchInterval: liveRefreshMs(live, range),
    refetchIntervalInBackground: false,
  }
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
    base.sort,
  ])
  const cursors = usePageCursors(scope, byOffset)
  const requested = search.page ?? 1
  const page = cursors.reachable(requested) ? requested : 1
  const params = traceSearchParamsWithCursor(
    { ...base, page },
    byOffset ? undefined : cursors.cursorFor(page)
  )
  const future = rangeInFuture(range, openedAt)
  const results = useQuery({
    ...exploreQuery(datasource.uid, params),
    ...polling,
    enabled: !future,
  })
  const { ready: statsReady } = useAfterMainQuery(results, scope)
  const stats = useQuery({
    ...traceStatsQuery(datasource.uid, {
      query: traceStatsFilter(datasource.type, search),
      ...range,
    }),
    ...polling,
    enabled: !future && statsReady && hasTraceStats(datasource.type),
  })
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

  if (future) {
    return (
      <p role="status" className="text-[13px]">
        <StatusLabel tone="warning">
          This time range starts in the future, so no trace can match. Pick an
          earlier range.
        </StatusLabel>
      </p>
    )
  }
  if (results.fetchStatus === "paused" && !data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  const failureText = results.isError
    ? traceQLProblem(exploreErrorText(results.error), base.query)
    : null
  const shorter = failureText === null ? null : shorterRange(failureText)
  const failure =
    failureText === null ? null : (
      <div className="flex flex-col gap-2">
        <ExploreFailure
          text={failureText}
          onRetry={() => void results.refetch()}
        />
        {shorter !== null ? (
          <Button
            type="button"
            variant="outline"
            size="xs"
            className="self-start"
            onClick={() => update({ from: shorter.from, to: "now" })}
          >
            {shorter.label}
          </Button>
        ) : null}
      </div>
    )
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
      <div className="flex flex-col gap-3">
        {failure}
        <p
          role={meta.partial ? "status" : undefined}
          className="text-[13px] text-muted-foreground"
        >
          {noTracesText(
            meta.partial,
            lowerFirst(describeRange(range.from, range.to))
          )}
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
  const limited = meta.partial || state.hasNext || state.page > 1
  const order = listOrder(
    datasource.type === "clickhouse" ? decodeSort(search.sort) : null
  )
  const covered = limited && order.newest ? partialNote(rows, undefined) : null
  return (
    <div className="flex flex-col gap-3">
      {failure}
      {lost !== null && lost.scope === scope ? (
        <p role="status" className="text-[13px] text-muted-foreground">
          Page {lost.page} could not be restored; showing page 1
        </p>
      ) : null}
      {stats.isError ? (
        <p role="status" className="text-[13px]">
          <StatusLabel tone="critical">
            Stats failed:{" "}
            {traceQLProblem(exploreErrorText(stats.error), base.query)}
          </StatusLabel>
        </p>
      ) : null}
      {limited ? (
        <div
          role="status"
          className="flex flex-wrap items-center gap-3 rounded-card border py-2 pr-2 pl-3 text-[13px]"
        >
          <StatusLabel tone="warning">Partial</StatusLabel>
          <p className="min-w-0 flex-1">
            {partialReason({
              order: order.text,
              rowCount: rows.length,
              pageSize,
              hasNext: state.hasNext,
              cut: meta.partial,
            })}
            {covered !== null ? ` ${covered.text}` : null}
          </p>
          {covered !== null ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-auto min-h-7 max-w-full py-1 text-left font-normal whitespace-normal tabular-nums narrow:min-h-11"
              onClick={() => update({ from: covered.from, to: covered.to })}
            >
              Keep only {covered.covers}
            </Button>
          ) : null}
        </div>
      ) : null}
      <div className="flex flex-col gap-1">
        <SelectableTraceTable
          key={`${scope}:${state.page}`}
          datasource={datasource}
          search={search}
          update={update}
          rows={rows}
          p95={p95}
          label={label}
          fitWindow={limited && order.newest}
        />
        {pageBar}
      </div>
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
  fitWindow,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
  update: UpdateSearch
  rows: TraceRow[]
  p95: number | null | undefined
  label: string
  fitWindow: boolean
}) {
  const range = exploreRange(search)
  const panel = tracesPanelFor({
    id: "traces",
    title: tracePanelTitle(search),
    datasource: datasource.uid,
    type: datasource.type,
    query: tracePanelQuery(datasource.type, search),
    y: 0,
  })
  const [selection, setSelection] = useState<Selection | null>(null)
  const sortControl: TraceSortControl | undefined =
    datasource.type === "clickhouse"
      ? {
          value: decodeSort(search.sort),
          onChange: (next) => {
            setSelection(null)
            update({ sort: encodeSort(next) })
          },
        }
      : undefined

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
      fitWindow={fitWindow}
      sortControl={sortControl}
      headerActions={
        <>
          <AddToDashboardAction panel={panel} />
          <AddToObservationAction range={range} panel={panel} />
        </>
      }
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
            next={keepSelectionSearch(datasource.type, search, selection)}
            onKeep={keep}
          />
        )
      }
      selectionPanel={
        selection === null || !hasTraceStats(datasource.type) ? undefined : (
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
