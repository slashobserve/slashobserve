import {
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type KeyboardEvent,
  type ReactNode,
} from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"
import { DownloadIcon, LogsIcon } from "lucide-react"

import { ApiError } from "@/api/errors"
import { CopyButton } from "@/components/copy-button"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { downloadFile } from "@/dashboards/export-spec"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"
import {
  TimeRangeControl,
  type TimeRangeValue,
} from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { useDocumentTitle } from "@/lib/document-title"
import { tracesLink, traceUrl } from "@/lib/links"
import { lowerFirst } from "@/lib/text"
import { describeRange, zonedDateTimeMs } from "@/lib/time"
import { cn } from "@/lib/utils"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "@/logs/datasource-picker"
import { exploreErrorText, offlineText } from "@/logs/explore"
import { usePrepared } from "@/logs/prepared"
import { useElementSize } from "@/logs/use-element-size"
import { SidePanel } from "@/shell/side-panel"
import { TopRowActions } from "@/shell/top-row-actions"
import { TopRowTitle } from "@/shell/top-row-title"
import { foldGroups, foldThreshold } from "@/panels/traces/fold"
import {
  criticalPath,
  errorAncestors,
  filterSpans,
  spanFilters,
  type SpanFilters,
} from "@/panels/traces/span-filters"
import {
  traceModel,
  visibleSpans,
  type Span,
} from "@/panels/traces/trace-model"
import {
  kindLine,
  SpanDetails,
  TraceView,
  type SearchLink,
  type TraceViewProps,
} from "@/panels/traces/traces-panel"
import type { DataPanel } from "@/spec"

import { wholeTrace, type Zoom } from "./minimap"
import { TraceMinimap } from "./minimap.tsx"
import {
  normaliseTraceId,
  traceDatasourceTypes,
  traceViewRange,
  type TraceViewSearch,
} from "./search"
import { traceQLAttribute } from "./search-query"
import { TraceLogsSection } from "./trace-logs-tab"
import {
  criticalPathDisabledText,
  traceNotes,
  traceNotice,
  traceQuery,
} from "./trace-query"
import { ServiceLegend, TraceToolbar, type TraceService } from "./trace-toolbar"

type TraceModelData = TraceViewProps["model"]

const noFilters = spanFilters({})
const noIds: ReadonlySet<string> = new Set()
const noDatasources: readonly DatasourceStatus[] = []
const skeletonRows = [0, 1, 2, 3, 4, 5]
const phoneQuery = "(max-width: 899px)"
const pageMaxSpans = 20000
const toolbarHint = "Click a row for details. Drag on the ruler to zoom."
const foldedToolbarHint = `Find searches every loaded span. ${toolbarHint}`
const legendHint = "Click a service to dim its spans."
const foldedLegendHint = `${legendHint} Counts include folded spans.`
const keysHint =
  "Use j and k to move, Enter to open a span, Escape to close the details."
const foldedHint =
  "Folded rows keep the hidden spans' totals so the timeline still adds up. Open a folded row to see its slowest 50 spans; open the rest from there."

function subscribePhone(onChange: () => void) {
  const media = window.matchMedia(phoneQuery)
  media.addEventListener("change", onChange)
  return () => media.removeEventListener("change", onChange)
}

function usePhone(): boolean {
  return useSyncExternalStore(
    subscribePhone,
    () => window.matchMedia(phoneQuery).matches,
    () => false
  )
}

type SearchUpdate = (next: Partial<TraceViewSearch>, replace?: boolean) => void

function useTraceViewSearch() {
  const search = useSearch({ from: "/_authed/_shell/t/$traceId" })
  const navigate = useNavigate({ from: "/t/$traceId" })
  const update: SearchUpdate = (next, replace = false) =>
    void navigate({ search: (state) => ({ ...state, ...next }), replace })
  return { search, update }
}

export function TracePage({ traceId }: { traceId: string }) {
  const id = normaliseTraceId(traceId)
  if (id === null) return <InvalidTraceId />
  return <TraceForId id={id} />
}

function InvalidTraceId() {
  useDocumentTitle("Trace - SlashObserve")
  return (
    <>
      <TopRowTitle>
        <span aria-current="page" className="min-w-0 truncate font-medium">
          Trace
        </span>
      </TopRowTitle>
      <PageHeader title="Trace" />
      <p className="text-[13px] text-muted-foreground">
        This is not a trace id. A trace id has 16 or 32 hexadecimal characters.
      </p>
    </>
  )
}

function TraceForId({ id }: { id: string }) {
  const ids = useId()
  const { search, update } = useTraceViewSearch()
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, traceDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  const range = traceViewRange(search)
  const actions: PageActions = {
    time: <TimeRangeControl value={range} onChange={update} />,
    share: datasource ? (
      <CopyButton
        variant="ghost"
        size="icon"
        text={
          window.location.origin +
          traceUrl(id, datasource.uid, range, search.span)
        }
        label="Copy link"
      />
    ) : (
      <Link to="/traces" className={cn(buttonVariants({ variant: "outline" }))}>
        Search traces
      </Link>
    ),
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-3">
      {datasources.isPending ? (
        <>
          <TraceHeader id={id} datasource={datasource} actions={actions} />
          <TraceSkeleton />
        </>
      ) : datasource ? (
        <TraceResult
          datasource={datasource}
          id={id}
          search={search}
          update={update}
          actions={actions}
        />
      ) : (
        <>
          <TraceHeader id={id} datasource={datasource} actions={actions} />
          <div className="flex flex-col gap-3">
            <ExploreDatasourcePicker
              id={`${ids}-datasource`}
              types={traceDatasourceTypes}
              value={undefined}
              onChange={(ds) => update({ ds })}
              label="Datasource"
            />
            <p className="text-[13px] text-muted-foreground">
              {eligible.length === 0
                ? "No traces datasource is configured. An Admin adds a Tempo or ClickHouse datasource in the datasources file."
                : "Pick a Tempo or ClickHouse datasource to open this trace."}
            </p>
          </div>
        </>
      )}
    </div>
  )
}

type PageActions = { time: ReactNode; share: ReactNode }
type TraceActions = { logs: ReactNode; download: ReactNode }

function TraceHeader({
  id,
  datasource,
  model,
  traceActions,
  actions,
}: {
  id: string
  datasource?: DatasourceStatus
  model?: TraceModelData
  traceActions?: TraceActions
  actions: PageActions
}) {
  const title = model
    ? [model.rootService, model.rootOperation].filter(Boolean).join(" ")
    : ""
  useDocumentTitle(
    title
      ? `${title} - Trace - SlashObserve`
      : `Trace ${id.slice(0, 8)} - SlashObserve`
  )
  return (
    <>
      <TopRowTitle>
        {datasource ? (
          <>
            <Link
              {...tracesLink(datasource.uid)}
              className="min-w-0 truncate rounded-control text-muted-foreground no-underline outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            >
              {datasource.name}
            </Link>
            <span aria-hidden="true" className="text-muted-foreground">
              /
            </span>
          </>
        ) : null}
        <span aria-current="page" className="min-w-0 truncate font-medium">
          {title || "Trace"}
        </span>
      </TopRowTitle>
      <TopRowActions>
        {actions.time}
        {traceActions?.logs}
        {actions.share}
        {traceActions?.download}
      </TopRowActions>
      <h1 className="sr-only text-xl font-semibold tracking-[-0.01em] narrow:not-sr-only">
        {title || "Trace"}
      </h1>
      <div className="flex flex-wrap items-center gap-x-5 gap-y-1 text-[13px]">
        {model ? <ModelStats model={model} /> : null}
        <Stat label="Trace">
          <span className="font-mono text-xs">{id.slice(0, 8)}</span>
          <CopyButton
            variant="ghost"
            size="icon-xs"
            text={id}
            label="Copy trace id"
          />
        </Stat>
      </div>
    </>
  )
}

function selfMsOf(spans: readonly Span[]): Map<string, number> {
  const childDurations = new Map<string, number>()
  for (const span of spans) {
    if (span.parentId === null) continue
    childDurations.set(
      span.parentId,
      (childDurations.get(span.parentId) ?? 0) + span.duration
    )
  }
  return new Map(
    spans.map((span) => [
      span.id,
      Math.max(span.duration - (childDurations.get(span.id) ?? 0), 0),
    ])
  )
}

function serviceList(spans: readonly Span[]): TraceService[] {
  const self = selfMsOf(spans)
  const services = new Map<string, TraceService>()
  for (const span of spans) {
    const service = services.get(span.service) ?? {
      name: span.service,
      slot: span.slot,
      spans: 0,
      selfMs: 0,
    }
    service.spans += 1
    service.selfMs += self.get(span.id) ?? 0
    services.set(span.service, service)
  }
  return [...services.values()]
}

function slowestOnPath(
  spans: readonly Span[],
  path: ReadonlySet<string>
): { service: string; ms: number } | null {
  const self = selfMsOf(spans)
  const byService = new Map<string, number>()
  for (const span of spans) {
    if (!path.has(span.id)) continue
    byService.set(
      span.service,
      (byService.get(span.service) ?? 0) + (self.get(span.id) ?? 0)
    )
  }
  let slowest: { service: string; ms: number } | null = null
  for (const [service, ms] of byService) {
    if (slowest === null || ms > slowest.ms) slowest = { service, ms }
  }
  return slowest
}

function Stat({ label, children }: { label: string; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </span>
  )
}

function ModelStats({ model }: { model: TraceModelData }) {
  const errors = model.spans.filter((span) => span.error).length
  const services = new Set(model.spans.map((span) => span.service)).size
  const slowest = useMemo(
    () => slowestOnPath(model.spans, criticalPath(model.spans)),
    [model.spans]
  )
  return (
    <>
      <Stat label="Duration">
        <span className="tabular-nums">
          {model.formatDuration(model.duration)}
        </span>
      </Stat>
      <Stat label="Spans">
        <span className="tabular-nums">
          {model.spans.length.toLocaleString("en")}
        </span>
      </Stat>
      <Stat label="Services">
        <span className="tabular-nums">{services}</span>
      </Stat>
      <Stat label="Errors">
        <span
          className={cn("tabular-nums", errors > 0 && "text-status-critical")}
        >
          {errors.toLocaleString("en")}
        </span>
      </Stat>
      <Stat label="Started">
        <time
          dateTime={new Date(model.start).toISOString()}
          className="tabular-nums"
        >
          {zonedDateTimeMs(model.start, undefined)}
        </time>
      </Stat>
      {slowest && slowest.service ? (
        <Stat label="Slowest service">
          <span>
            {slowest.service},{" "}
            <span className="tabular-nums">
              {model.formatDuration(slowest.ms)}
            </span>{" "}
            on the critical path
          </span>
        </Stat>
      ) : null}
    </>
  )
}

function TraceSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2">
      {skeletonRows.map((row) => (
        <Skeleton key={row} className="h-6" />
      ))}
    </div>
  )
}

function TraceResult({
  datasource,
  id,
  search,
  update,
  actions,
}: {
  datasource: DatasourceStatus
  id: string
  search: TraceViewSearch
  update: SearchUpdate
  actions: PageActions
}) {
  const range = traceViewRange(search)
  const result = useQuery(traceQuery(datasource.uid, id, range))
  const missing = (
    <TraceMissing
      id={id}
      datasource={datasource}
      range={range}
      update={update}
      actions={actions}
    />
  )
  if (result.fetchStatus === "paused" && !result.data) {
    return (
      <>
        <TraceHeader id={id} datasource={datasource} actions={actions} />
        <p className="text-[13px] text-muted-foreground">{offlineText}</p>
      </>
    )
  }
  if (result.isError) {
    if (result.error instanceof ApiError && result.error.status === 404) {
      return missing
    }
    return (
      <>
        <TraceHeader id={id} datasource={datasource} actions={actions} />
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {exploreErrorText(result.error)}
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => void result.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      </>
    )
  }
  if (!result.data) {
    return (
      <>
        <TraceHeader id={id} datasource={datasource} actions={actions} />
        <TraceSkeleton />
      </>
    )
  }
  return (
    <TraceWaterfall
      id={id}
      datasource={datasource}
      range={range}
      data={result.data}
      missing={missing}
      openId={search.span ?? null}
      onOpenIdChange={(span) => update({ span: span ?? undefined }, true)}
      actions={actions}
    />
  )
}

function TraceMissing({
  id,
  datasource,
  range,
  update,
  actions,
}: {
  id: string
  datasource: DatasourceStatus
  range: TimeRangeValue
  update: SearchUpdate
  actions: PageActions
}) {
  return (
    <>
      <TraceHeader id={id} datasource={datasource} actions={actions} />
      <div className="min-h-64 min-w-0 flex-1 rounded-card border border-border">
        <div className="flex flex-col items-start gap-2 p-4 text-[13px] text-muted-foreground">
          <p>
            No spans for this trace in{" "}
            {lowerFirst(describeRange(range.from, range.to))}. Widen the time
            range.
          </p>
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => update({ from: "now-7d", to: "now" })}
          >
            Last 7 days
          </Button>
        </div>
      </div>
    </>
  )
}

function tempoSearchLink(uid: string): SearchLink {
  return (attribute, negate) => ({
    uid,
    q: traceQLAttribute(
      attribute.source,
      attribute.key,
      attribute.text,
      negate
    ),
  })
}

function isFormField(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLElement &&
    target.closest("input, textarea, select, [contenteditable]") !== null
  )
}

function downloadTrace(id: string, data: PanelDataResponse) {
  downloadFile(
    new Blob([JSON.stringify(data, null, 2) + "\n"], {
      type: "application/json",
    }),
    `trace-${id}.json`
  )
}

function TraceWaterfall({
  id,
  datasource,
  range,
  data,
  missing,
  openId,
  onOpenIdChange,
  actions,
}: {
  id: string
  datasource: DatasourceStatus
  range: TimeRangeValue
  data: PanelDataResponse
  missing: ReactNode
  openId: string | null
  onOpenIdChange: (id: string | null) => void
  actions: PageActions
}) {
  const panel = useMemo<DataPanel>(
    () => ({
      id: "trace",
      type: "traces",
      title: "Trace",
      datasource: datasource.uid,
      gridPos: { x: 0, y: 0, w: 24, h: 20 },
      queries: [{ query: "" }],
    }),
    [datasource.uid]
  )
  const frames: WireFrame[] = data.frames
  const prepared = usePrepared(panel, frames)
  const model = useMemo(() => {
    const built = traceModel(prepared.frames, prepared.fields, {
      maxSpans: pageMaxSpans,
    })
    if (built.reason !== null) return built
    return { ...built, notes: [...traceNotes(data.meta), ...built.notes] }
  }, [prepared.frames, prepared.fields, data.meta])
  const searchLink = useMemo(
    () =>
      datasource.type === "tempo" ? tempoSearchLink(datasource.uid) : undefined,
    [datasource.type, datasource.uid]
  )
  if (model.reason !== null || model.spans.length === 0) return missing
  return (
    <TraceExplorer
      id={id}
      datasource={datasource}
      range={range}
      meta={data.meta}
      model={model}
      searchLink={searchLink}
      openId={openId}
      onOpenIdChange={onOpenIdChange}
      onDownload={() => downloadTrace(id, data)}
      actions={actions}
    />
  )
}

function TraceExplorer({
  id,
  datasource,
  range,
  meta,
  model,
  searchLink,
  openId,
  onOpenIdChange,
  onDownload,
  actions,
}: {
  id: string
  datasource: DatasourceStatus
  range: TimeRangeValue
  meta: PanelDataResponse["meta"]
  model: TraceModelData
  searchLink: SearchLink | undefined
  openId: string | null
  onOpenIdChange: (id: string | null) => void
  onDownload: () => void
  actions: PageActions
}) {
  const { ref, width, height } = useElementSize<HTMLDivElement>()
  const attributes = usePhone() ? "inline" : "external"
  const detailsId = useId()
  const searchRef = useRef<HTMLInputElement>(null)
  const [search, setSearch] = useState("")
  const [showPath, setShowPath] = useState(false)
  const [errorsOnly, setErrorsOnly] = useState(false)
  const [hiddenServices, setHiddenServices] =
    useState<ReadonlySet<string>>(noIds)
  const [zoom, setZoom] = useState<Zoom>(wholeTrace)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(noIds)
  const [logsOpen, setLogsOpen] = useState(false)
  const logsTrigger = useRef<HTMLButtonElement>(null)
  const openedFrom = useRef<HTMLElement | null>(null)
  const closedSpan = useRef<string | null>(null)
  const datasources = useDatasources()
  const all = datasources.data ?? noDatasources
  const spans = model.spans

  const filters = useMemo<SpanFilters>(
    () => ({
      ...noFilters,
      search: search.trim(),
      serviceNames: [...hiddenServices],
      excludeServices: true,
    }),
    [search, hiddenServices]
  )
  const filtered = useMemo(() => filterSpans(spans, filters), [spans, filters])
  const pathIds = useMemo(() => criticalPath(spans), [spans])
  const dimmed = useMemo(() => {
    if (!errorsOnly) return undefined
    const kept = errorAncestors(spans)
    return new Set(
      spans.filter((span) => !kept.has(span.id)).map((span) => span.id)
    )
  }, [spans, errorsOnly])
  const services = useMemo(() => serviceList(spans), [spans])
  const groups = useMemo(() => foldGroups(spans, foldThreshold), [spans])
  const folded = groups.size > 0
  const notice = traceNotice(meta, spans.length, folded)
  const rows = useMemo(() => visibleSpans(spans, collapsed), [spans, collapsed])
  const matchIds = useMemo(() => {
    const matched = filtered.matched
    if (filters.search === "" || matched === null) return []
    return rows.filter((span) => matched.has(span.id)).map((span) => span.id)
  }, [rows, filtered.matched, filters.search])
  const matchIndex = openId === null ? -1 : matchIds.indexOf(openId)
  const openSpan = useMemo(
    () =>
      openId === null ? undefined : spans.find((span) => span.id === openId),
    [spans, openId]
  )
  const spanPanelOpen =
    attributes === "external" && openSpan !== undefined && !logsOpen

  const changeOpenId = (next: string | null) => {
    if (next === null) {
      closedSpan.current = attributes === "external" ? openId : null
    } else {
      if (openId === null) {
        openedFrom.current =
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null
      }
      closedSpan.current = null
      setLogsOpen(false)
    }
    onOpenIdChange(next)
  }
  const openLogs = () => {
    onOpenIdChange(null)
    setLogsOpen(true)
  }

  useEffect(() => {
    if (spanPanelOpen || closedSpan.current === null) return
    const row = ref.current?.querySelector<HTMLElement>(
      `[data-span-id="${CSS.escape(closedSpan.current)}"]`
    )
    closedSpan.current = null
    row?.focus()
  }, [spanPanelOpen, ref])

  const openMatch = (step: 1 | -1) => {
    if (matchIds.length === 0) return
    const next =
      matchIndex === -1
        ? step === 1
          ? 0
          : matchIds.length - 1
        : (matchIndex + step + matchIds.length) % matchIds.length
    changeOpenId(matchIds[next])
  }
  const toggleService = (name: string) =>
    setHiddenServices((current) => {
      const next = new Set(current)
      if (!next.delete(name)) next.add(name)
      return next
    })

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key !== "/" || event.metaKey || event.ctrlKey || event.altKey)
      return
    if (isFormField(event.target)) return
    event.preventDefault()
    searchRef.current?.focus()
  }

  const status = [
    showPath
      ? `Critical path on: the ${pathIds.size.toLocaleString("en")} outlined spans add up to the ${model.formatDuration(model.duration)} total; the ${(spans.length - pathIds.size).toLocaleString("en")} dimmed spans ran alongside them.`
      : "",
    folded ? foldedToolbarHint : toolbarHint,
    folded ? foldedHint : "",
    keysHint,
  ]
    .filter(Boolean)
    .join(" ")

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3" onKeyDown={onKeyDown}>
      <TraceHeader
        id={id}
        datasource={datasource}
        model={model}
        traceActions={{
          logs: (
            <Button
              ref={logsTrigger}
              type="button"
              variant="outline"
              onClick={openLogs}
            >
              <LogsIcon aria-hidden="true" data-icon="inline-start" />
              Logs for this trace
            </Button>
          ),
          download: (
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Download JSON"
              title="Download JSON"
              onClick={onDownload}
            >
              <DownloadIcon aria-hidden="true" />
            </Button>
          ),
        }}
        actions={actions}
      />
      {notice !== null ? (
        <p
          role="status"
          className="rounded-card border border-border p-3 text-[13px]"
        >
          <StatusLabel tone="warning">{notice}</StatusLabel>
        </p>
      ) : null}
      <TraceToolbar
        search={search}
        onSearchChange={setSearch}
        searchRef={searchRef}
        matchIndex={matchIndex === -1 ? null : matchIndex}
        matchCount={matchIds.length}
        criticalPath={showPath}
        onCriticalPathChange={setShowPath}
        criticalPathDisabled={criticalPathDisabledText(meta)}
        errorsOnly={errorsOnly}
        onErrorsOnlyChange={setErrorsOnly}
        onCollapseAll={() =>
          setCollapsed(
            new Set(
              spans.filter((span) => span.hasChildren).map((span) => span.id)
            )
          )
        }
        onExpandAll={() => setCollapsed(noIds)}
        onPrevMatch={() => openMatch(-1)}
        onNextMatch={() => openMatch(1)}
      />
      <div
        ref={ref}
        className="min-h-64 min-w-0 flex-1 rounded-card border border-border"
      >
        {width > 0 && height > 0 ? (
          <TraceView
            model={model}
            filters={filters}
            zone={undefined}
            width={width}
            height={height}
            attributes={attributes}
            detailsId={detailsId}
            openId={openId}
            onOpenIdChange={changeOpenId}
            collapsed={collapsed}
            onCollapsedChange={setCollapsed}
            toolbar={
              <>
                <ServiceLegend
                  services={services}
                  hiddenServices={hiddenServices}
                  onToggleService={toggleService}
                  hint={folded ? foldedLegendHint : legendHint}
                  formatDuration={model.formatDuration}
                />
                <TraceMinimap model={model} zoom={zoom} onZoom={setZoom} />
              </>
            }
            highlighted={filtered.matched}
            criticalPathIds={showPath ? pathIds : undefined}
            dimmed={dimmed}
            zoom={zoom}
            onZoom={setZoom}
            header={false}
            fold={folded ? groups : undefined}
          />
        ) : null}
      </div>
      <p aria-live="polite" className="text-xs text-muted-foreground">
        {status}
      </p>
      {spanPanelOpen ? (
        <SidePanel
          title={openSpan.operation || openSpan.id}
          description={kindLine(openSpan)}
          onClose={() => changeOpenId(null)}
          initialFocus={openedFrom}
        >
          <div id={detailsId}>
            <SpanDetails
              span={openSpan}
              model={model}
              searchLink={searchLink}
              traceLinks={{
                ds: datasource.uid,
                from: range.from,
                to: range.to,
              }}
              extra={
                <TraceLogsSection
                  traceUid={datasource.uid}
                  traceId={id}
                  span={openSpan}
                  trace={model}
                  all={all}
                  preview
                  onOpenTraceLogs={openLogs}
                />
              }
            />
          </div>
        </SidePanel>
      ) : null}
      {logsOpen ? (
        <SidePanel
          title="Logs for this trace"
          description={id.slice(0, 8)}
          onClose={() => setLogsOpen(false)}
          trigger={logsTrigger}
        >
          <TraceLogsSection
            traceUid={datasource.uid}
            traceId={id}
            trace={model}
            all={all}
            preview={false}
          />
        </SidePanel>
      ) : null}
    </div>
  )
}
