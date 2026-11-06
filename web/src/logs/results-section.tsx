import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/empty-state"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import type { PanelDataMeta } from "@/dashboards/queries"
import { useDatasources } from "@/datasources/queries"
import { lowerFirst } from "@/lib/text"
import { describeRange } from "@/lib/time"

import { typeName } from "./datasource-picker"
import { exploreErrorText, offlineText } from "./explore"
import { rowCount } from "./frames"
import { LogsResults } from "./logs-results"
import { PatternsView } from "./patterns-view"
import { isSearchFamily } from "./search"
import { streamLimit, streamLimitText } from "./stream-limit"
import { traceDatasourceFor } from "./trace-link"
import { useElementSize } from "./use-element-size"
import { resultRows, useLogsWorkspace } from "./workspace"

const skeletonRows = [0, 1, 2, 3, 4]

function count(value: number): string {
  return value.toLocaleString("en")
}

function hitsText(meta: PanelDataMeta, type: string): string {
  const noun = isSearchFamily(type) ? "hits" : "lines"
  const shown =
    meta.truncated && meta.rows_total !== undefined
      ? `${count(meta.rows)} of ${count(meta.rows_total)}`
      : count(meta.rows)
  const took =
    meta.elapsed_ms === undefined
      ? ""
      : ` in ${(meta.elapsed_ms / 1000).toFixed(1)} s`
  return `${shown} ${noun}${took}`
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
    range,
    results,
    tail,
    mode,
    update,
    openContext,
    labels,
  } = useLogsWorkspace()
  const allDatasources = useDatasources()
  const { ref, height } = useElementSize<HTMLDivElement>()
  const q = search.q
  const patterns = search.view === "patterns"
  const frames = tail.shown ? tail.buffer?.frames : results.data?.frames
  const lines = frames ? rowCount(frames) : 0
  const traceDatasource = traceDatasourceFor(
    datasource,
    allDatasources.data ?? []
  )
  const meta = !tail.shown ? results.data?.meta : undefined
  const hits = meta === undefined ? null : hitsText(meta, datasource.type)

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
    if (!tail.shown && results.fetchStatus === "paused" && !results.data) {
      return (
        <p className="p-3 text-[13px] text-muted-foreground">{offlineText}</p>
      )
    }
    if (!tail.shown && results.isError) {
      const limit = streamLimit(results.error)
      return (
        <div className="p-3">
          <Alert variant="destructive">
            <AlertDescription className="flex flex-col items-start gap-2">
              {limit !== null
                ? streamLimitText(limit)
                : exploreErrorText(results.error)}
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={
                  limit !== null
                    ? () => labels.setOpen(true)
                    : () => void results.refetch()
                }
              >
                {limit !== null ? "Open label browser" : "Retry"}
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
      <LogsResults
        frames={frames}
        datasource={datasource}
        onContext={openContext}
        traceDatasource={traceDatasource}
        height={height}
      />
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
          onValueChange={(next) =>
            update({ view: next === "patterns" ? "patterns" : undefined })
          }
        >
          <TabsList variant="line">
            <TabsTrigger value="documents">Documents</TabsTrigger>
            <TabsTrigger value="patterns">Patterns</TabsTrigger>
          </TabsList>
        </Tabs>
        {hits === null ? null : (
          <p className="text-xs text-muted-foreground">{hits}</p>
        )}
      </div>
      {hits === null ? null : (
        <p className="border-b border-border px-3 py-1.5 text-xs text-muted-foreground">
          {[typeName(datasource.type), datasource.name, datasource.index, hits]
            .filter((part) => part !== undefined)
            .join(" ")}
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
      {lines === resultRows && !patterns ? (
        <p className="border-t border-border px-3 py-1.5 text-xs text-muted-foreground">
          Showing the newest 1,000 lines. Narrow the query or the time range.
        </p>
      ) : null}
    </section>
  )
}
