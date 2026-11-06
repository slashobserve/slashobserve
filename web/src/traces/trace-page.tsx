import { useId, useMemo, useSyncExternalStore } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"

import { CopyButton } from "@/components/copy-button"
import { PageHeader } from "@/components/page-header"
import { StatusLabel } from "@/components/status-label"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { WireFrame } from "@/dashboards/queries"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { traceUrl } from "@/lib/links"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"
import { cn } from "@/lib/utils"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "@/logs/datasource-picker"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { usePrepared } from "@/logs/prepared"
import { useElementSize } from "@/logs/use-element-size"
import { spanFilters } from "@/panels/traces/span-filters"
import { traceModel } from "@/panels/traces/trace-model"
import { TraceView, type TraceViewProps } from "@/panels/traces/traces-panel"
import type { DataPanel } from "@/spec"

import {
  normaliseTraceId,
  traceDatasourceTypes,
  traceViewRange,
  type TraceViewSearch,
} from "./search"
import { traceByIdParams } from "./search-query"
import { countText } from "./trace-rows"

const noFilters = spanFilters({})
const skeletonRows = [0, 1, 2, 3, 4, 5]
const phoneQuery = "(max-width: 760px)"

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

function useTraceViewSearch() {
  const search = useSearch({ from: "/_authed/_shell/t/$traceId" })
  const navigate = useNavigate({ from: "/t/$traceId" })
  const update = (next: Partial<TraceViewSearch>) =>
    void navigate({ search: (state) => ({ ...state, ...next }) })
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

  return (
    <>
      <PageHeader
        title="Trace"
        description={
          <span className="inline-flex items-center gap-1">
            <span className="font-mono">{id.slice(0, 8)}</span>
            <CopyButton
              variant="ghost"
              size="icon-xs"
              text={id}
              label="Copy trace id"
            />
          </span>
        }
        actions={
          <>
            <TimeRangeControl value={range} onChange={update} />
            {datasource ? (
              <CopyButton
                variant="outline"
                text={
                  window.location.origin + traceUrl(id, datasource.uid, range)
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
        }
      />
      {datasources.isPending ? (
        <TraceSkeleton />
      ) : datasource ? (
        <TraceResult datasource={datasource} id={id} search={search} />
      ) : (
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
      )}
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
}: {
  datasource: DatasourceStatus
  id: string
  search: TraceViewSearch
}) {
  const range = traceViewRange(search)
  const result = useQuery(
    exploreQuery(datasource.uid, traceByIdParams(datasource.type, id, range))
  )
  if (result.fetchStatus === "paused" && !result.data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  if (result.isError) {
    return (
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
    )
  }
  if (!result.data) return <TraceSkeleton />
  const missing =
    datasource.type === "tempo"
      ? "Tempo has no trace with this id."
      : `No spans for this trace in ${lowerFirst(describeRange(range.from, range.to))}. Widen the time range.`
  return (
    <TraceWaterfall
      datasourceUid={datasource.uid}
      frames={result.data.frames}
      missing={missing}
    />
  )
}

function TraceWaterfall({
  datasourceUid,
  frames,
  missing,
}: {
  datasourceUid: string
  frames: WireFrame[]
  missing: string
}) {
  const panel = useMemo<DataPanel>(
    () => ({
      id: "trace",
      type: "traces",
      title: "Trace",
      datasource: datasourceUid,
      gridPos: { x: 0, y: 0, w: 24, h: 20 },
      queries: [{ query: "" }],
    }),
    [datasourceUid]
  )
  const prepared = usePrepared(panel, frames)
  const model = useMemo(
    () => traceModel(prepared.frames, prepared.fields),
    [prepared.frames, prepared.fields]
  )
  const { ref, width, height } = useElementSize<HTMLDivElement>()
  const attributes = usePhone() ? "inline" : "pane"
  const empty = model.reason !== null || model.spans.length === 0
  return (
    <div className="flex flex-col gap-2">
      {model.reason === null && !empty ? <TraceSummary model={model} /> : null}
      <div
        ref={ref}
        className="h-[calc(100dvh-18rem)] min-h-96 min-w-0 rounded-card border border-border"
      >
        {empty ? (
          <p className="p-4 text-[13px] text-muted-foreground">{missing}</p>
        ) : width > 0 && height > 0 ? (
          <TraceView
            model={model}
            filters={noFilters}
            zone={undefined}
            width={width}
            height={height}
            attributes={attributes}
          />
        ) : null}
      </div>
    </div>
  )
}

function TraceSummary({ model }: { model: TraceViewProps["model"] }) {
  const errors = model.spans.filter((span) => span.error).length
  const parts = [
    model.rootService,
    model.rootOperation,
    model.formatDuration(model.duration),
    countText(model.spans.length, "span"),
  ].filter(Boolean)
  return (
    <p className="flex flex-wrap items-center gap-x-3 text-[13px]">
      <span>{parts.join(" · ")}</span>
      {errors > 0 ? (
        <StatusLabel tone="critical">{countText(errors, "error")}</StatusLabel>
      ) : null}
    </p>
  )
}
