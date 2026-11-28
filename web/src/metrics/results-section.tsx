import { useMemo, useState, type ReactNode } from "react"

import { EmptyState } from "@/components/empty-state"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { PanelDataResponse } from "@/dashboards/queries"
import type { TimeRangeValue } from "@/dashboards/time-range-picker"
import { offlineText } from "@/logs/explore"
import { useElementSize } from "@/lib/use-element-size"
import { describeRange } from "@/lib/time"
import { lowerFirst } from "@/lib/text"
import { PanelBody } from "@/panels/panel-body"
import { createSharedCursor } from "@/panels/shared-cursor"

import { metricsPanelFor } from "./panel"
import { truncationNote } from "./problems"
import { QueryFailure } from "./query-failure"
import type { MetricsMode } from "./search"
import { useMetricsResults } from "./workspace"

const chartHeight = 420
const noVars = {}

function ignoreSelect() {}

function tookText(elapsedMs: number | undefined): string {
  return elapsedMs === undefined ? "" : ` in ${(elapsedMs / 1000).toFixed(1)} s`
}

function stepText(stepMs: number): string {
  if (stepMs <= 0) return ""
  return stepMs % 60_000 === 0
    ? `, step ${stepMs / 60_000} m`
    : `, step ${stepMs / 1000} s`
}

export function summaryText(data: PanelDataResponse, mode: MetricsMode) {
  if (mode === "table") {
    const rows = data.meta.rows
    return `${rows.toLocaleString("en")} ${rows === 1 ? "row" : "rows"}${tookText(data.meta.elapsed_ms)}`
  }
  const series = data.frames.length
  return `${series.toLocaleString("en")} series${stepText(data.meta.step_ms)}${tookText(data.meta.elapsed_ms)}`
}

function ResultSkeleton() {
  return (
    <div aria-busy="true" className="flex flex-col gap-2 p-3">
      <Skeleton className="h-4 w-1/3 motion-safe:animate-none" />
      <Skeleton className="h-40 w-full motion-safe:animate-none" />
    </div>
  )
}

export function ResultsSection({
  uid,
  query,
  range,
  mode,
  onMode,
  onSelectRange,
  actions,
}: {
  uid: string
  query: string | undefined
  range: TimeRangeValue
  mode: MetricsMode
  onMode: (mode: MetricsMode) => void
  onSelectRange: (fromMs: number, toMs: number) => void
  actions: ReactNode
}) {
  const { ref, width } = useElementSize<HTMLDivElement>()
  const [cursor] = useState(createSharedCursor)
  const results = useMetricsResults({ uid, query, range, mode, width })
  const panel = useMemo(
    () =>
      metricsPanelFor({
        id: "explore",
        title: "Explore",
        datasource: uid,
        query: query ?? "",
        mode,
        y: 0,
      }),
    [uid, query, mode]
  )
  const data = results.data
  const empty =
    data !== undefined &&
    (mode === "table" ? data.meta.rows === 0 : data.frames.length === 0)

  function body(): ReactNode {
    if (query === undefined) {
      return (
        <EmptyState title="Write a query and press Run">
          Start from a metric name in the browser, or write an expression such
          as rate(http_requests_total[5m]).
        </EmptyState>
      )
    }
    if (results.fetchStatus === "paused" && data === undefined) {
      return (
        <p className="p-3 text-[13px] text-muted-foreground">{offlineText}</p>
      )
    }
    if (results.isError && data === undefined) {
      return (
        <div className="flex flex-col gap-2 p-4">
          <QueryFailure
            uid={uid}
            query={query}
            error={results.error}
            onRetry={() => void results.refetch()}
          />
        </div>
      )
    }
    if (data === undefined) return <ResultSkeleton />
    if (empty) {
      return (
        <EmptyState title="No data in this range">
          The query matched no series in{" "}
          {lowerFirst(describeRange(range.from, range.to))}. Widen the time
          range or loosen the label matchers.
        </EmptyState>
      )
    }
    return (
      <PanelBody
        panel={panel}
        data={data}
        width={Math.max(0, width)}
        height={chartHeight}
        stateScope="explore:metrics"
        cursor={cursor}
        onTimeRangeSelect={mode === "graph" ? onSelectRange : ignoreSelect}
        vars={noVars}
        timezone={undefined}
        from={range.from}
        to={range.to}
      />
    )
  }

  const note = data === undefined ? null : truncationNote(data.meta.truncated)
  return (
    <section
      aria-label="Results"
      aria-busy={results.isFetching}
      className="flex min-w-0 flex-1 flex-col"
    >
      <div className="flex min-h-11 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border px-4 py-1.5">
        <Tabs
          value={mode}
          onValueChange={(next) => onMode(next === "table" ? "table" : "graph")}
        >
          <TabsList className="h-7! rounded-control p-0.5 narrow:h-11!">
            <TabsTrigger value="graph" className="rounded-chip text-[13px]">
              Graph
            </TabsTrigger>
            <TabsTrigger value="table" className="rounded-chip text-[13px]">
              Table
            </TabsTrigger>
          </TabsList>
        </Tabs>
        {data === undefined ? null : (
          <p className="text-[13px] text-muted-foreground tabular-nums narrow:order-last narrow:w-full">
            {summaryText(data, mode)}
          </p>
        )}
        <div className="ml-auto flex flex-wrap items-center gap-2">
          {actions}
        </div>
      </div>
      {note === null ? null : (
        <p className="border-b border-border px-4 py-1.5 text-xs text-muted-foreground">
          {note}
        </p>
      )}
      <div
        ref={ref}
        role="tabpanel"
        aria-label={mode === "table" ? "Table" : "Graph"}
        className="relative min-h-[26rem] min-w-0 flex-1"
      >
        {body()}
      </div>
    </section>
  )
}
