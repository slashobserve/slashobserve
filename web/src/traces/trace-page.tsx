import {
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

import { ApiError } from "@/api/errors"
import { CopyButton } from "@/components/copy-button"
import { PageHeader } from "@/components/page-header"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { PanelDataResponse, WireFrame } from "@/dashboards/queries"
import {
  TimeRangeControl,
  type TimeRangeValue,
} from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { traceUrl } from "@/lib/links"
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
import { traceNotes, traceQuery } from "./trace-query"
import { TraceToolbar, type TraceService } from "./trace-toolbar"

type TraceModelData = TraceViewProps["model"]

const noFilters = spanFilters({})
const noIds: ReadonlySet<string> = new Set()
const skeletonRows = [0, 1, 2, 3, 4, 5]
const phoneQuery = "(max-width: 760px)"
const toolbarHint = "Click a row for details. Drag on the ruler to zoom."
const legendHint = "Click a service to dim its spans."
const keysHint =
  "Use j and k to move, Enter to open a span, Escape to close the details."

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
  if (id === null) {
    return (
      <>
        <PageHeader title="Trace" />
        <p className="text-[13px] text-muted-foreground">
          This is not a trace id. A trace id has 16 or 32 hexadecimal
          characters.
        </p>
      </>
    )
  }
  return <TraceForId id={id} />
}

function TraceForId({ id }: { id: string }) {
  const ids = useId()
  const { search, update } = useTraceViewSearch()
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, traceDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  const range = traceViewRange(search)
  const actions = (
    <>
      <TimeRangeControl value={range} onChange={update} />
      {datasource ? (
        <CopyButton
          variant="outline"
          text={
            window.location.origin +
            traceUrl(id, datasource.uid, range, search.span)
          }
          label="Copy link"
        >
          Copy link
        </CopyButton>
      ) : null}
      <Link
        to="/traces"
        search={{ ds: datasource?.uid }}
        className={cn(buttonVariants({ variant: "outline" }))}
      >
        Search traces
      </Link>
    </>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {datasources.isPending ? (
        <>
          <TraceHeader id={id} actions={actions} />
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
          <TraceHeader id={id} actions={actions} />
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

function TraceHeader({
  id,
  model,
  actions,
}: {
  id: string
  model?: TraceModelData
  actions: ReactNode
}) {
  const title = model
    ? [model.rootService, model.rootOperation].filter(Boolean).join(" ")
    : ""
  return (
    <PageHeader
      title={title || "Trace"}
      description={
        model ? (
          <TraceStats model={model} />
        ) : (
          <span className="inline-flex items-center gap-1">
            <span className="font-mono">{id.slice(0, 8)}</span>
            <CopyButton
              variant="ghost"
              size="icon-xs"
              text={id}
              label="Copy trace id"
            />
          </span>
        )
      }
      actions={
        <>
          {model ? (
            <CopyButton
              variant="outline"
              text={id}
              label={`Copy trace id ${id}`}
            >
              Copy trace id
            </CopyButton>
          ) : null}
          {actions}
        </>
      }
    />
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
    <span className="inline-flex gap-1.5">
      <span className="text-muted-foreground">{label}</span>
      {children}
    </span>
  )
}

function TraceStats({ model }: { model: TraceModelData }) {
  const errors = model.spans.filter((span) => span.error).length
  const services = new Set(model.spans.map((span) => span.service)).size
  const slowest = useMemo(
    () => slowestOnPath(model.spans, criticalPath(model.spans)),
    [model.spans]
  )
  return (
    <span className="flex flex-wrap gap-x-4 gap-y-1 text-[13px] text-foreground">
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
    </span>
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
  actions: ReactNode
}) {
  const range = traceViewRange(search)
  const result = useQuery(traceQuery(datasource.uid, id, range))
  const missing = (
    <TraceMissing id={id} range={range} update={update} actions={actions} />
  )
  if (result.fetchStatus === "paused" && !result.data) {
    return (
      <>
        <TraceHeader id={id} actions={actions} />
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
        <TraceHeader id={id} actions={actions} />
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
        <TraceHeader id={id} actions={actions} />
        <TraceSkeleton />
      </>
    )
  }
  return (
    <TraceWaterfall
      id={id}
      datasource={datasource}
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
  range,
  update,
  actions,
}: {
  id: string
  range: TimeRangeValue
  update: SearchUpdate
  actions: ReactNode
}) {
  return (
    <>
      <TraceHeader id={id} actions={actions} />
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
    ds: uid,
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

function TraceWaterfall({
  id,
  datasource,
  data,
  missing,
  openId,
  onOpenIdChange,
  actions,
}: {
  id: string
  datasource: DatasourceStatus
  data: PanelDataResponse
  missing: ReactNode
  openId: string | null
  onOpenIdChange: (id: string | null) => void
  actions: ReactNode
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
    const built = traceModel(prepared.frames, prepared.fields)
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
      model={model}
      searchLink={searchLink}
      openId={openId}
      onOpenIdChange={onOpenIdChange}
      actions={actions}
    />
  )
}

function TraceExplorer({
  id,
  model,
  searchLink,
  openId,
  onOpenIdChange,
  actions,
}: {
  id: string
  model: TraceModelData
  searchLink: SearchLink | undefined
  openId: string | null
  onOpenIdChange: (id: string | null) => void
  actions: ReactNode
}) {
  const { ref, width, height } = useElementSize<HTMLDivElement>()
  const attributes = usePhone() ? "inline" : "pane"
  const searchRef = useRef<HTMLInputElement>(null)
  const [search, setSearch] = useState("")
  const [showPath, setShowPath] = useState(false)
  const [errorsOnly, setErrorsOnly] = useState(false)
  const [hiddenServices, setHiddenServices] =
    useState<ReadonlySet<string>>(noIds)
  const [zoom, setZoom] = useState<Zoom>(wholeTrace)
  const [collapsed, setCollapsed] = useState<ReadonlySet<string>>(noIds)
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
  const rows = useMemo(() => visibleSpans(spans, collapsed), [spans, collapsed])
  const matchIds = useMemo(() => {
    const matched = filtered.matched
    if (filters.search === "" || matched === null) return []
    return rows.filter((span) => matched.has(span.id)).map((span) => span.id)
  }, [rows, filtered.matched, filters.search])
  const matchIndex = openId === null ? -1 : matchIds.indexOf(openId)

  const openMatch = (step: 1 | -1) => {
    if (matchIds.length === 0) return
    const next =
      matchIndex === -1
        ? step === 1
          ? 0
          : matchIds.length - 1
        : (matchIndex + step + matchIds.length) % matchIds.length
    onOpenIdChange(matchIds[next])
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

  const status = showPath
    ? `Critical path on: the ${pathIds.size.toLocaleString("en")} outlined spans add up to the ${model.formatDuration(model.duration)} total; the ${(spans.length - pathIds.size).toLocaleString("en")} dimmed spans ran alongside them. ${keysHint}`
    : keysHint

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-2" onKeyDown={onKeyDown}>
      <TraceHeader id={id} model={model} actions={actions} />
      <TraceToolbar
        search={search}
        onSearchChange={setSearch}
        searchRef={searchRef}
        matchIndex={matchIndex === -1 ? null : matchIndex}
        matchCount={matchIds.length}
        criticalPath={showPath}
        onCriticalPathChange={setShowPath}
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
        hint={toolbarHint}
        services={services}
        hiddenServices={hiddenServices}
        onToggleService={toggleService}
        legendHint={legendHint}
        onPrevMatch={() => openMatch(-1)}
        onNextMatch={() => openMatch(1)}
        formatDuration={model.formatDuration}
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
            openId={openId}
            onOpenIdChange={onOpenIdChange}
            collapsed={collapsed}
            onCollapsedChange={setCollapsed}
            toolbar={
              <TraceMinimap model={model} zoom={zoom} onZoom={setZoom} />
            }
            highlighted={filtered.matched}
            criticalPathIds={showPath ? pathIds : undefined}
            dimmed={dimmed}
            zoom={zoom}
            onZoom={setZoom}
            header={false}
            searchLink={searchLink}
          />
        ) : null}
      </div>
      <p aria-live="polite" className="text-xs text-muted-foreground">
        {status}
      </p>
    </div>
  )
}
