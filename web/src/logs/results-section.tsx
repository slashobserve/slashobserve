import {
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ComponentProps,
  type UIEvent,
} from "react"

import { EmptyState } from "@/components/empty-state"
import { PageBar } from "@/components/page-bar"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"
import { useDatasources } from "@/datasources/queries"
import {
  defaultPageSize,
  pagesByOffset,
  pageStateOf,
  rangeText,
  windowNote,
  type PageState,
} from "@/lib/paging"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import type { LogEntry } from "@/panels/logs/logs-model"
import type { LogSelection } from "@/panels/logs/logs-panel"

import { typeName } from "./datasource-picker"
import { exploreErrorText, offlineText } from "./explore"
import {
  keysOf,
  oldestFirstTimes,
  rowCount,
  timesWhere,
  withoutKeys,
  type TailBuffer,
} from "./frames"
import { columnList, columnsText } from "./line-fields"
import { LogsResults, type ResultsView } from "./logs-results"
import { PatternsTabLabel, PatternsView } from "./patterns-view"
import { isSearchFamily } from "./search"
import {
  arrivalMarkMs,
  recentlyArrived,
  tailExpired,
  tailPaused,
  tailReceived,
  tailResumed,
  tailStarted,
  tailStatusText,
  type TailState,
} from "./tail-state"
import { TailVolume } from "./tail-volume"
import { traceDatasourceFor } from "./trace-link"
import { useElementSize } from "./use-element-size"
import { tailRows } from "./use-live-tail"
import { useLogsWorkspace } from "./workspace"

const skeletonRows = [0, 1, 2, 3, 4]

function count(value: number): string {
  return value.toLocaleString("en")
}

function tookText(elapsedMs: number | undefined): string {
  return elapsedMs === undefined ? "" : ` in ${(elapsedMs / 1000).toFixed(1)} s`
}

function hitsText(
  data: PanelDataResponse,
  state: PageState,
  type: string
): string {
  const noun = isSearchFamily(type) ? "hits" : "lines"
  const shown =
    state.totalRelation === "eq" && state.total !== undefined
      ? state.total
      : data.meta.rows
  const page = state.page > 1 ? `, page ${state.page}` : ""
  return `${count(shown)} ${noun}${tookText(data.meta.elapsed_ms)}${page}`
}

function sourceText(
  data: PanelDataResponse,
  state: PageState,
  type: string,
  name: string,
  index: string | undefined
): string {
  const noun = isSearchFamily(type) ? "hits" : "lines"
  return [
    typeName(type),
    name,
    index,
    rangeText(state, noun, tookText(data.meta.elapsed_ms)),
  ]
    .filter((part) => part !== undefined)
    .join(" ")
}

const lokiPagingNote =
  "Loki pages by time from the last line shown, so there is no page count."

function pagingNote(type: string, state: PageState): string | undefined {
  if (type === "loki") return lokiPagingNote
  if (isSearchFamily(type))
    return windowNote(state, typeName(type)) ?? undefined
  return undefined
}

type TailViewState = { tail: TailState; now: number }

type TailAction =
  | { type: "received"; keys: string[]; kept: ReadonlySet<string>; now: number }
  | { type: "paused" }
  | { type: "resumed"; now: number }
  | { type: "expired"; now: number }

function reduceTail(state: TailViewState, action: TailAction): TailViewState {
  switch (action.type) {
    case "received":
      return {
        tail: tailReceived(state.tail, action.keys, action.now, action.kept),
        now: action.now,
      }
    case "paused":
      return { ...state, tail: tailPaused(state.tail) }
    case "resumed":
      return { tail: tailResumed(state.tail, action.now), now: action.now }
    case "expired":
      return { ...state, tail: tailExpired(state.tail, action.now) }
  }
}

const bottomSlackPx = 8
const emptyKeys: ReadonlySet<string> = new Set()
const emptyTimes: ReadonlySet<number> = new Set()
const listSelector = "ul[data-logs-count]"

function capitalized(text: string): string {
  return `${text[0].toUpperCase()}${text.slice(1)}`
}

function TailBody({
  buffer,
  noun,
  streaming,
  results,
}: {
  buffer: TailBuffer
  noun: string
  streaming: boolean
  results: Omit<
    ComponentProps<typeof LogsResults>,
    | "frames"
    | "height"
    | "listTitle"
    | "orderLocked"
    | "onRendered"
    | "isMarked"
  >
}) {
  const { ref: regionRef, height } = useElementSize<HTMLDivElement>()
  const [state, dispatch] = useReducer(reduceTail, buffer, (initial) => ({
    tail: tailStarted([...initial.keys]),
    now: initial.startedAt ?? 0,
  }))
  const [frozen, setFrozen] = useState<WireFrame[] | null>(null)
  const [cleared, setCleared] = useState(emptyKeys)
  const following = useRef(true)
  const latest = useRef(buffer.frames)

  useEffect(() => {
    latest.current = buffer.frames
    dispatch({
      type: "received",
      keys: buffer.added,
      kept: buffer.keys,
      now: Date.now(),
    })
  }, [buffer])

  const marking = state.tail.arrivedAt
  useEffect(() => {
    if (marking.size === 0) return
    const timer = setTimeout(
      () => dispatch({ type: "expired", now: Date.now() }),
      arrivalMarkMs
    )
    return () => clearTimeout(timer)
  }, [marking])

  const live = frozen ?? buffer.frames
  const shown = useMemo(() => withoutKeys(live, cleared), [live, cleared])
  const lines = rowCount(shown)
  const times = useMemo(() => oldestFirstTimes(buffer.frames), [buffer.frames])
  const arrivedTimes = useMemo(
    () =>
      marking.size === 0
        ? emptyTimes
        : timesWhere(shown, (key) => recentlyArrived(state.tail, key)),
    [shown, marking, state.tail]
  )
  const isMarked = useCallback(
    (entry: LogEntry) => entry.time !== null && arrivedTimes.has(entry.time),
    [arrivedTimes]
  )

  const scroller = useCallback(
    () =>
      regionRef.current?.querySelector<HTMLElement>(listSelector)
        ?.parentElement ?? null,
    [regionRef]
  )
  const toBottom = useCallback(() => {
    const element = scroller()
    if (element) element.scrollTop = element.scrollHeight
  }, [scroller])
  const follow = useCallback(() => {
    if (following.current) toBottom()
  }, [toBottom])
  useEffect(follow)

  function pause() {
    following.current = false
    setFrozen(latest.current)
    dispatch({ type: "paused" })
  }

  function resume() {
    following.current = true
    setFrozen(null)
    dispatch({ type: "resumed", now: Date.now() })
    toBottom()
  }

  function onScroll(event: UIEvent<HTMLDivElement>) {
    const element = event.target
    if (
      !(element instanceof HTMLElement) ||
      !element.firstElementChild?.matches(listSelector)
    ) {
      return
    }
    const away = element.scrollHeight - element.scrollTop - element.clientHeight
    if (away > bottomSlackPx && following.current) pause()
    else if (away <= bottomSlackPx && !following.current) resume()
  }

  const paused = state.tail.paused
  const pending = state.tail.pendingCount
  const singular = noun.slice(0, -1)
  const pendingNoun = pending === 1 ? singular : noun
  return (
    <>
      <div className="flex h-7 shrink-0 items-center justify-between gap-2 px-3 text-[13px] text-muted-foreground">
        <p>{`Newest ${lines.toLocaleString("en")} ${noun}, oldest at the top`}</p>
        <div className="flex items-center gap-1">
          {paused ? (
            <Button type="button" variant="ghost" size="xs" onClick={resume}>
              Resume
            </Button>
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="xs"
            aria-label="Clear the lines shown"
            onClick={() => setCleared(keysOf(live))}
          >
            Clear
          </Button>
        </div>
      </div>
      <p className="h-7 shrink-0 px-3 text-[13px] text-muted-foreground">
        {tailStatusText(
          buffer.startedAt ?? 0,
          buffer.received,
          tailRows,
          noun,
          streaming
        )}
      </p>
      <TailVolume times={times} now={state.now} noun={noun} />
      <div
        ref={regionRef}
        className="relative min-h-0 flex-1"
        onScrollCapture={onScroll}
      >
        {lines === 0 ? (
          <p className="p-3 text-[13px] text-muted-foreground">
            No {noun} yet. New {noun} appear here as they arrive.
          </p>
        ) : height > 0 ? (
          <LogsResults
            {...results}
            frames={shown}
            height={height}
            listTitle="Live tail, oldest first"
            orderLocked
            onRendered={follow}
            isMarked={isMarked}
          />
        ) : null}
        {paused && pending > 0 ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            aria-label={`${pending} new ${pendingNoun} arrived while paused. Jump to the newest and resume`}
            className="absolute bottom-3 left-1/2 -translate-x-1/2 bg-popover"
            onClick={resume}
          >
            {`${pending.toLocaleString("en")} new ${pendingNoun}, jump to newest`}
          </Button>
        ) : null}
      </div>
      <p className="shrink-0 border-t border-border px-3 py-1.5 text-xs text-muted-foreground">
        {paused
          ? `Paused because you scrolled up. Resume to follow new ${noun}. `
          : ""}
        {capitalized(noun)} older than 5 minutes drop off the top
      </p>
      <p aria-live="polite" className="sr-only">
        {paused && pending > 0
          ? `${pending} new ${pendingNoun} while paused`
          : ""}
      </p>
    </>
  )
}

export function ResultSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {skeletonRows.map((row) => (
        <Skeleton key={row} className="h-5" />
      ))}
    </div>
  )
}

export function ResultsSection() {
  const {
    datasource,
    search,
    effectiveQuery,
    range,
    results,
    tail,
    mode,
    update,
    detail,
    paging,
  } = useLogsWorkspace()
  const allDatasources = useDatasources()
  const [view, setView] = useState<ResultsView>({
    order: "newest",
    wrap: false,
    prettify: false,
    columnSort: null,
  })
  const onViewChange = useCallback(
    (change: Partial<ResultsView>) =>
      setView((current) => ({ ...current, ...change })),
    []
  )
  const columns = useMemo(() => columnList(search.cols), [search.cols])
  const onColumnsChange = useCallback(
    (names: string[]) => {
      setView((current) =>
        current.columnSort !== null && !names.includes(current.columnSort.name)
          ? { ...current, columnSort: null }
          : current
      )
      update({ cols: columnsText(names) }, { replace: true })
    },
    [update]
  )
  const onLinkedChange = useCallback(
    (line: string | undefined) => update({ line }, { replace: true }),
    [update]
  )
  const { ref, height } = useElementSize<HTMLDivElement>()
  const q = search.q
  const patterns = search.view === "patterns"
  const resultsScope = `${effectiveQuery ?? ""}|${range.from}|${range.to}|${paging.pageSize}`
  const [lastGood, setLastGood] = useState<{
    scope: string
    data: PanelDataResponse
  } | null>(null)
  const fresh =
    results.data !== undefined && !results.isPlaceholderData
      ? results.data
      : undefined
  if (fresh !== undefined && lastGood?.data !== fresh) {
    setLastGood({ scope: resultsScope, data: fresh })
  }
  const kept =
    results.isError && lastGood?.scope === resultsScope
      ? lastGood.data
      : undefined
  const data = results.data ?? kept
  const frames = data?.frames
  const lines = frames ? rowCount(frames) : 0
  const traceDatasource = traceDatasourceFor(
    datasource,
    allDatasources.data ?? []
  )
  const byOffset = pagesByOffset(datasource.type)
  const shownData = !tail.shown ? data : undefined
  const pageState =
    shownData === undefined
      ? null
      : pageStateOf(shownData.meta.page, lines, byOffset, {
          page: paging.page,
          pageSize: paging.pageSize,
          truncated: shownData.meta.truncated,
        })
  const hits =
    shownData === undefined || pageState === null
      ? null
      : hitsText(shownData, pageState, datasource.type)

  function resultsProps() {
    return {
      datasource,
      traceDatasource,
      selectedKey: detail.selection?.entry.key ?? null,
      onSelect: detail.select,
      onContext: (selection: LogSelection) =>
        detail.select(selection, "context"),
      detailsId: detail.drawerId,
      view: tail.shown
        ? { ...view, order: "oldest" as const, columnSort: null }
        : view,
      onViewChange,
      columns,
      onColumnsChange,
      linkedId: search.line,
      onLinkedChange,
    }
  }

  function body() {
    if (patterns) return <PatternsView />
    if (q === undefined) {
      if (mode === "ask") return null
      return (
        <EmptyState title="Write a query and press Run">
          {datasource.type === "loki"
            ? 'Pick a label to start, or write a stream selector such as {service_name="checkout"}.'
            : null}
        </EmptyState>
      )
    }
    if (tail.shown && tail.buffer !== undefined) {
      return (
        <TailBody
          key={tail.buffer.startedAt}
          buffer={tail.buffer}
          noun={isSearchFamily(datasource.type) ? "documents" : "lines"}
          streaming={tail.on}
          results={resultsProps()}
        />
      )
    }
    if (!tail.shown && results.fetchStatus === "paused" && !results.data) {
      return (
        <p className="p-3 text-[13px] text-muted-foreground">{offlineText}</p>
      )
    }
    if (!tail.shown && results.isError && kept === undefined) {
      return (
        <div className="p-3">
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
        </div>
      )
    }
    if (frames === undefined) {
      return (
        <div className="p-3">
          <ResultSkeleton />
        </div>
      )
    }
    if (lines === 0) {
      return (
        <p className="p-3 text-[13px] text-muted-foreground">
          No lines in {lowerFirst(describeRange(range.from, range.to))}. Widen
          the time range or loosen the query.
        </p>
      )
    }
    return height > 0 ? (
      <LogsResults frames={frames} height={height} {...resultsProps()} />
    ) : null
  }

  return (
    <section
      aria-label="Results"
      aria-busy={results.isFetching || tail.isFetching}
      className="flex min-w-0 flex-1 flex-col rounded-card border border-border"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 border-b border-border px-3 py-2">
        <Tabs
          value={patterns ? "patterns" : "documents"}
          onValueChange={(next) => {
            if (next === "patterns") tail.stop()
            update({ view: next === "patterns" ? "patterns" : undefined })
          }}
        >
          <TabsList variant="line">
            <TabsTrigger value="documents">Documents</TabsTrigger>
            <span title={q === undefined ? "Run a query first." : undefined}>
              <TabsTrigger value="patterns" disabled={q === undefined}>
                <PatternsTabLabel />
              </TabsTrigger>
            </span>
          </TabsList>
        </Tabs>
        {hits === null ? null : (
          <p className="text-xs text-muted-foreground">{hits}</p>
        )}
      </div>
      {hits === null ? null : (
        <p className="border-b border-border px-3 py-1.5 text-xs text-muted-foreground">
          {shownData === undefined || pageState === null
            ? null
            : sourceText(
                shownData,
                pageState,
                datasource.type,
                datasource.name,
                datasource.index
              )}
        </p>
      )}
      {tail.error ? (
        <div className="p-3">
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-2">
              Live tail stopped: {lowerFirst(exploreErrorText(tail.error))}
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={tail.start}
              >
                Start again
              </Button>
            </AlertDescription>
          </Alert>
        </div>
      ) : null}
      <div
        ref={ref}
        role="tabpanel"
        aria-label={patterns ? "Patterns" : "Documents"}
        className="relative min-h-0 flex-1 overflow-auto"
      >
        <div className="absolute inset-0 flex flex-col">{body()}</div>
      </div>
      {pageState !== null &&
      shownData !== undefined &&
      !patterns &&
      (pageState.rows > 0 || pageState.hasPrev) ? (
        <div className="flex flex-col gap-2 border-t border-border px-3 py-2">
          {results.isError && kept !== undefined ? (
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
          ) : null}
          {paging.lostPage !== null ? (
            <p role="status" className="text-[13px] text-muted-foreground">
              Page {paging.lostPage} could not be restored; showing page 1
            </p>
          ) : null}
          {shownData.meta.partial && isSearchFamily(datasource.type) ? (
            <StatusLabel tone="warning">
              Some shards did not answer; counts may be low
            </StatusLabel>
          ) : null}
          <PageBar
            state={pageState}
            noun={isSearchFamily(datasource.type) ? "hits" : "lines"}
            qualifier=" in range"
            note={pagingNote(datasource.type, pageState)}
            label="Document pages"
            busy={results.isFetching}
            onPage={(next) => update({ page: next })}
            onPageSize={(size) =>
              update({ size: size === defaultPageSize ? undefined : size })
            }
          />
        </div>
      ) : null}
    </section>
  )
}
