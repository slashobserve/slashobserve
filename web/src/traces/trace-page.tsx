import { useId, useMemo } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"

import { CopyButton } from "@/components/copy-button"
import { PageHeader } from "@/components/page-header"
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
import { TraceView } from "@/panels/traces/traces-panel"
import type { DataPanel } from "@/spec"

import {
  normaliseTraceId,
  traceDatasourceTypes,
  traceViewRange,
  type TraceViewSearch,
} from "./search"
import { traceByIdParams } from "./search-query"

const noFilters = spanFilters({})
const skeletonRows = [0, 1, 2, 3, 4, 5]

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
        title={`Trace ${id.slice(0, 8)}`}
        description={<span className="font-mono">{id}</span>}
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
  const empty = model.reason !== null || model.spans.length === 0
  return (
    <div
      ref={ref}
      className="h-[calc(100dvh-16rem)] min-h-96 min-w-0 rounded-card border border-border"
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
          attributes="pane"
        />
      ) : null}
    </div>
  )
}
