import {
  useEffect,
  useId,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"
import {
  BookmarkIcon,
  CircleAlertIcon,
  FolderSearchIcon,
  MoreHorizontalIcon,
} from "lucide-react"

import { AskField } from "@/ask/ask-field"
import { nextMode, useInputMode, type InputMode } from "@/ask/input-mode"
import { InputModeToggle } from "@/ask/input-mode-toggle"
import { useAsk } from "@/ask/use-ask"
import { useInputModeShortcut } from "@/ask/use-input-mode-shortcut"
import { AgentMark } from "@/components/agent-mark"
import { PageBar } from "@/components/page-bar"
import { LoadProblem } from "@/components/load-problem"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { PanelDataResponse } from "@/dashboards/queries"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useSourceMemory } from "@/datasources/datasource-memory"
import type { DatasourceStatus } from "@/datasources/queries"
import { SourceTitle } from "@/datasources/source-title"
import { useExploreSources } from "@/datasources/use-sources"
import { useDocumentTitle } from "@/lib/document-title"
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
import { cn } from "@/lib/utils"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { ExploreFailure } from "@/logs/explore-failure"
import { usePhone } from "@/logs/logs-results"
import { useAgentInfo } from "@/observe/queries"
import { tracesDraft } from "@/saved-searches/draft"
import { savedSearchLocation } from "@/saved-searches/links"
import { savedSearchesQuery } from "@/saved-searches/queries"
import { SaveSearchDialog } from "@/saved-searches/save-search-dialog"
import { SavedSearchesMenu } from "@/saved-searches/saved-searches-menu"
import { loadProblemText } from "@/service-accounts/queries"
import { TopRowActions } from "@/shell/top-row-actions"

import { traceSearchFromAnswer } from "./ask-answer"
import { ComparePanel } from "./compare-panel"
import type { Selection } from "./scatter"
import { tracesTimeRange, type TraceSearch } from "./search"
import {
  attributeFilter,
  keepSelectionSearch,
  traceSearchParams,
  traceSearchParamsWithCursor,
  traceStatsFilter,
} from "./search-query"
import { traceFilterChips } from "./trace-filters"
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

const traceModes: readonly InputMode[] = ["ask", "code"]
const traceModeLabels: Record<InputMode, string> = {
  ask: "Ask",
  builder: "Builder",
  code: "Query",
}

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
  const range = tracesTimeRange(search)
  const [saving, setSaving] = useState(false)
  const saveTrigger = useRef<HTMLButtonElement>(null)

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
          No traces datasource is configured. An Admin adds a Tempo or
          ClickHouse datasource in the datasources file.
        </p>
      </>
    )
  }
  if (datasource === undefined) {
    return (
      <>
        <h1 className="sr-only">Traces</h1>
        <p className="text-[13px] text-muted-foreground">
          No trace source has the uid {uid}.
        </p>
        <Link
          to="/traces"
          className="mt-2 inline-block text-[13px] underline underline-offset-3"
        >
          Open Traces
        </Link>
      </>
    )
  }

  return (
    <div className="flex flex-col gap-3">
      <h1 className="sr-only">Traces in {datasource.name}</h1>
      <SourceTitle kind="traces" source={datasource} />
      <TraceSearchActions
        range={range}
        onRangeChange={update}
        saveTrigger={saveTrigger}
        onSave={() => setSaving(true)}
      />
      <SaveSearchDialog
        draft={tracesDraft({
          uid: datasource.uid,
          type: datasource.type,
          search,
          range,
        })}
        open={saving}
        onOpenChange={setSaving}
        finalFocus={() => saveTrigger.current ?? true}
      />
      <TraceSearchWorkspace
        key={datasource.uid}
        datasource={datasource}
        search={search}
        update={update}
      />
    </div>
  )
}

function SavedSearchItems() {
  const navigate = useNavigate()
  const query = useQuery(savedSearchesQuery({ kind: "traces" }))
  const items = query.data?.saved_searches ?? []
  if (query.isPending) {
    return <DropdownMenuItem disabled>Loading...</DropdownMenuItem>
  }
  if (query.isError) {
    return (
      <DropdownMenuItem disabled>
        {loadProblemText(query.error)}
      </DropdownMenuItem>
    )
  }
  if (items.length === 0) {
    return <DropdownMenuItem disabled>No saved searches yet.</DropdownMenuItem>
  }
  return items.map((saved) => (
    <DropdownMenuItem
      key={saved.id}
      onClick={() => void navigate(savedSearchLocation(saved))}
    >
      <span className="min-w-0 flex-1 truncate">{saved.title}</span>
    </DropdownMenuItem>
  ))
}

function TraceSearchActions({
  range,
  onRangeChange,
  saveTrigger,
  onSave,
}: {
  range: { from: string; to: string }
  onRangeChange: UpdateSearch
  saveTrigger: RefObject<HTMLButtonElement | null>
  onSave: () => void
}) {
  const phone = usePhone()
  return (
    <TopRowActions>
      <div className="flex items-center gap-2">
        <span className={cn("contents", phone && "[&_button]:h-11")}>
          <TimeRangeControl value={range} onChange={onRangeChange} />
        </span>
        {phone ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  ref={saveTrigger}
                  variant="ghost"
                  size="icon"
                  aria-label="More actions"
                  className="size-11"
                />
              }
            >
              <MoreHorizontalIcon aria-hidden="true" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuItem className="min-h-11" onClick={onSave}>
                <BookmarkIcon aria-hidden="true" />
                Save search
              </DropdownMenuItem>
              <DropdownMenuSub>
                <DropdownMenuSubTrigger className="min-h-11">
                  <FolderSearchIcon aria-hidden="true" />
                  Saved searches
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-64">
                  <SavedSearchItems />
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <>
            <span aria-hidden="true" className="h-5 w-px bg-border" />
            <SavedSearchesMenu kind="traces" />
            <Button
              ref={saveTrigger}
              type="button"
              variant="ghost"
              onClick={onSave}
            >
              <BookmarkIcon aria-hidden="true" />
              <span className="@max-[780px]/top:sr-only">Save search</span>
            </Button>
          </>
        )}
      </div>
    </TopRowActions>
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
      setMode("code", false)
      focusNext("code")
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

  const chips = traceFilterChips(search)
  const explanation = ask.answer?.explanation ?? ""
  const askRow = mode === "ask"

  return (
    <>
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
            mode={mode}
            modes={traceModes}
            labels={traceModeLabels}
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
              onClick={() => switchMode("code")}
            >
              Edit the query
            </Button>
          </div>
        ) : null}
        {askRow ? null : (
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
                  update(next)
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
                  className="narrow:h-11"
                  onClick={askAgain}
                >
                  Ask again
                </Button>
              ) : null}
            </div>
          </>
        )}
        {ask.problem !== null ? (
          <div
            role="alert"
            className="flex flex-col items-start gap-1 border-t px-4 py-2.5 text-[13px]"
          >
            {ask.problem.kind === "cannot_translate" ? (
              <>
                <StatusLabel tone="critical">
                  I could not turn that into a query.
                </StatusLabel>
                <p className="text-muted-foreground">{ask.problem.reason}</p>
              </>
            ) : (
              <StatusLabel tone="critical">{ask.problem.text}</StatusLabel>
            )}
          </div>
        ) : null}
      </section>
      <TraceResults datasource={datasource} search={search} update={update} />
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
    <ExploreFailure
      text={exploreErrorText(results.error)}
      onRetry={() => void results.refetch()}
    />
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
      <div className="flex flex-col gap-3">
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
    <div className="flex flex-col gap-3">
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
        <div
          role="status"
          className="flex flex-wrap items-center gap-3 rounded-card border py-2 pr-2 pl-3 text-[13px]"
        >
          <StatusLabel tone="warning">Partial</StatusLabel>
          <p className="min-w-0 flex-1">
            Tempo stopped searching at its time limit, so older traces may be
            missing.
            {covered !== null ? ` ${covered.text}` : null}
          </p>
          {covered !== null ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="font-normal tabular-nums narrow:h-11"
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
