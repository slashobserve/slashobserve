import { useQuery } from "@tanstack/react-query"
import { useId, useMemo, useState } from "react"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { DatasourceStatus } from "@/datasources/queries"
import { cn } from "@/lib/utils"
import { exploreErrorText } from "@/logs/explore"

import {
  attributeKey,
  compareBody,
  compareDescription,
  compareNote,
  compareRows,
  percentText,
  shareText,
  traceCompareQuery,
  type TraceCompare,
  type TraceDifference,
} from "./compare"
import { selectedRows, type Selection } from "./scatter"
import type { TraceSearch } from "./search"
import type { TraceRow } from "./trace-rows"

const skeletonRows = [0, 1, 2]
const heading = "What is different about the selected traces"

function ShareBar({ share, className }: { share: number; className: string }) {
  return (
    <span
      aria-hidden="true"
      className="block h-1.5 overflow-hidden rounded-full bg-muted"
    >
      <span
        className={cn("block h-full", className)}
        style={{ width: `${share * 100}%` }}
      />
    </span>
  )
}

function DifferenceRow({
  difference,
  filter,
  onFilter,
}: {
  difference: TraceDifference
  filter: Partial<TraceSearch> | null
  onFilter: (next: Partial<TraceSearch>) => void
}) {
  return (
    <li className="grid grid-cols-[minmax(0,5fr)_minmax(0,4fr)_52px] items-center gap-x-3 border-t border-border py-[7px]">
      <div className="flex min-w-0 flex-col font-mono text-xs">
        <span className="truncate text-muted-foreground">
          {difference.attribute}
        </span>
        <span className="truncate font-medium">{difference.value}</span>
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)_2.5rem] items-center gap-x-2.5 gap-y-0.5 text-xs">
        <ShareBar share={difference.selected_share} className="bg-series-1" />
        <span className="text-right font-medium tabular-nums">
          {percentText(difference.selected_share)}
        </span>
        <ShareBar
          share={difference.baseline_share}
          className="bg-muted-foreground"
        />
        <span className="text-right text-muted-foreground tabular-nums">
          {percentText(difference.baseline_share)}
        </span>
        <span className="sr-only">{shareText(difference)}</span>
      </div>
      {filter !== null ? (
        <Button
          type="button"
          variant="ghost"
          size="xs"
          aria-label={`Filter the search to ${attributeKey(difference.attribute)} = ${difference.value}`}
          onClick={() => onFilter(filter)}
        >
          Filter
        </Button>
      ) : null}
    </li>
  )
}

function LegendSwatch({
  className,
  children,
}: {
  className: string
  children: string
}) {
  return (
    <span className="flex items-center gap-1.5">
      <span
        aria-hidden="true"
        className={cn("h-2 w-2.5 rounded-sm", className)}
      />
      {children}
    </span>
  )
}

function Differences({
  result,
  filterFor,
  onFilter,
}: {
  result: TraceCompare
  filterFor: (difference: TraceDifference) => Partial<TraceSearch> | null
  onFilter: (next: Partial<TraceSearch>) => void
}) {
  const [showWeak, setShowWeak] = useState(false)
  const { strong, weak } = compareRows(result)
  const listed = showWeak ? [...strong, ...weak] : strong
  return (
    <>
      <div className="flex flex-wrap items-center gap-x-3.5 gap-y-1 text-xs">
        <LegendSwatch className="bg-series-1">
          {`Selected (${result.selected.read.toLocaleString("en")})`}
        </LegendSwatch>
        <LegendSwatch className="bg-muted-foreground">
          {`Everything else (${result.baseline.read.toLocaleString("en")})`}
        </LegendSwatch>
      </div>
      {result.differences.length === 0 ? (
        <p className="text-[13px] text-muted-foreground">
          No attribute stands out in the selection.
        </p>
      ) : (
        <ul className="flex flex-col text-[13px]">
          <li
            aria-hidden="true"
            className="grid grid-cols-[minmax(0,5fr)_minmax(0,4fr)_52px] gap-x-3 pb-1 text-xs text-muted-foreground"
          >
            <span>Attribute</span>
            <span>Share of traces</span>
          </li>
          {listed.map((difference) => (
            <DifferenceRow
              key={`${difference.attribute}\u0000${difference.value}`}
              difference={difference}
              filter={filterFor(difference)}
              onFilter={onFilter}
            />
          ))}
        </ul>
      )}
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs text-muted-foreground">
        <p>{compareNote(result)}</p>
        {weak.length > 0 && !showWeak ? (
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={() => setShowWeak(true)}
          >
            Show {weak.length} weaker differences
          </Button>
        ) : null}
      </div>
    </>
  )
}

export function ComparePanel({
  datasource,
  rows,
  selection,
  range,
  filterFor,
  onFilter,
}: {
  datasource: DatasourceStatus
  rows: readonly TraceRow[]
  selection: Selection
  range: { from: string; to: string }
  filterFor: (difference: TraceDifference) => Partial<TraceSearch> | null
  onFilter: (next: Partial<TraceSearch>) => void
}) {
  const headingId = useId()
  const body = useMemo(
    () => compareBody(rows, selection, range),
    [rows, selection, range]
  )
  const comparable = body.selected.length > 0 && body.baseline.length > 0
  const compare = useQuery({
    ...traceCompareQuery(datasource.uid, body),
    enabled: comparable,
  })
  const selectedCount = selectedRows(rows, selection).length

  let content
  if (!comparable) {
    content = (
      <p className="text-[13px] text-muted-foreground">
        {body.selected.length === 0
          ? "Select at least one trace to compare."
          : "Every trace is selected, so there is nothing to compare against."}
      </p>
    )
  } else if (compare.isError) {
    content = (
      <Alert variant="destructive">
        <AlertDescription className="flex flex-col items-start gap-2">
          {exploreErrorText(compare.error)}
          <Button
            type="button"
            variant="outline"
            size="xs"
            onClick={() => void compare.refetch()}
          >
            Retry
          </Button>
        </AlertDescription>
      </Alert>
    )
  } else if (compare.data === undefined) {
    content = (
      <div aria-busy="true" className="flex flex-col gap-2">
        {skeletonRows.map((key) => (
          <Skeleton key={key} className="h-8" />
        ))}
      </div>
    )
  } else {
    content = (
      <Differences
        result={compare.data}
        filterFor={filterFor}
        onFilter={onFilter}
      />
    )
  }

  return (
    <section
      aria-labelledby={headingId}
      className="flex min-w-0 flex-col gap-2 rounded-card border bg-card px-4 py-3"
    >
      <div className="flex flex-col gap-0.5">
        <h2 id={headingId} className="text-[13px] font-semibold">
          {heading}
        </h2>
        {compare.data !== undefined && comparable ? (
          <p className="text-xs text-muted-foreground">
            {compareDescription(
              compare.data,
              selectedCount,
              rows.length - selectedCount
            )}
          </p>
        ) : null}
      </div>
      {content}
    </section>
  )
}
