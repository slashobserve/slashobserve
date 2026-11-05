import { useId } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link, useNavigate, useSearch } from "@tanstack/react-router"

import { PageHeader } from "@/components/page-header"
import { Cell, HeaderCell, TableCard } from "@/components/table-card"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { useDatasources, type DatasourceStatus } from "@/datasources/queries"
import { traceLink } from "@/lib/links"
import { lowerFirst } from "@/lib/text"
import { describeRange, zonedDateTimeMs } from "@/lib/time"
import {
  eligibleDatasources,
  ExploreDatasourcePicker,
} from "@/logs/datasource-picker"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { useDefaultDatasource } from "@/logs/logs-page"

import {
  traceDatasourceTypes,
  tracesTimeRange,
  type TraceSearch,
} from "./search"
import { traceSearchParams, traceSearchRows } from "./search-query"
import { TraceSearchForm } from "./trace-search-form"
import { traceRows, type TraceRow } from "./trace-rows"

const skeletonRows = [0, 1, 2, 3, 4]

function useTraceSearch() {
  const search = useSearch({ from: "/_authed/_shell/traces" })
  const navigate = useNavigate({ from: "/traces" })
  const update = (next: Partial<TraceSearch>) =>
    void navigate({ search: (state) => ({ ...state, ...next }) })
  return { search, update }
}

function durationText(ms: number | null): string {
  return ms === null
    ? "-"
    : `${ms.toLocaleString("en", { maximumFractionDigits: 1 })} ms`
}

export function TracesPage() {
  const ids = useId()
  const { search, update } = useTraceSearch()
  const datasources = useDatasources()
  const eligible = eligibleDatasources(datasources.data, traceDatasourceTypes)
  const datasource = eligible.find((candidate) => candidate.uid === search.ds)
  useDefaultDatasource(datasource, eligible)
  const range = tracesTimeRange(search)

  if (!datasources.isPending && eligible.length === 0) {
    return (
      <>
        <PageHeader title="Traces" />
        <p className="text-[13px] text-muted-foreground">
          No traces datasource is configured. An Admin adds a Tempo or
          ClickHouse datasource in the datasources file.
        </p>
      </>
    )
  }

  return (
    <>
      <PageHeader
        title="Traces"
        actions={<TimeRangeControl value={range} onChange={update} />}
      />
      <div className="flex flex-col gap-4">
        <ExploreDatasourcePicker
          id={`${ids}-datasource`}
          types={traceDatasourceTypes}
          value={datasource?.uid}
          onChange={(ds) => update({ ds })}
          label="Datasource"
        />
        {datasource ? (
          <>
            <TraceSearchForm
              key={JSON.stringify([
                datasource.uid,
                search.service,
                search.span,
                search.min,
                search.max,
                search.errors,
                search.q,
              ])}
              type={datasource.type}
              uid={datasource.uid}
              value={search}
              onSubmit={update}
            />
            <TraceResults datasource={datasource} search={search} />
          </>
        ) : (
          <ResultSkeleton />
        )}
      </div>
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

function TraceResults({
  datasource,
  search,
}: {
  datasource: DatasourceStatus
  search: TraceSearch
}) {
  const range = tracesTimeRange(search)
  const results = useQuery(
    exploreQuery(datasource.uid, traceSearchParams(datasource.type, search))
  )

  if (results.fetchStatus === "paused" && !results.data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  if (results.isError) {
    return (
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
    )
  }
  if (!results.data) return <ResultSkeleton />
  const rows = traceRows(results.data.frames)
  if (rows.length === 0) {
    return (
      <p className="text-[13px] text-muted-foreground">
        No traces match in {lowerFirst(describeRange(range.from, range.to))}.
      </p>
    )
  }
  return (
    <div className="flex flex-col gap-2">
      {rows.length >= traceSearchRows ? (
        <p className="text-[13px] text-muted-foreground">
          Showing the newest 100 traces. Narrow the search.
        </p>
      ) : null}
      <TraceTable
        rows={rows}
        datasourceUid={datasource.uid}
        spans={datasource.type === "clickhouse"}
        range={range}
      />
    </div>
  )
}

function TraceTable({
  rows,
  datasourceUid,
  spans,
  range,
}: {
  rows: TraceRow[]
  datasourceUid: string
  spans: boolean
  range: { from: string; to: string }
}) {
  return (
    <TableCard label="Traces" minWidth={720}>
      <thead>
        <tr>
          <HeaderCell>Trace</HeaderCell>
          <HeaderCell>Started</HeaderCell>
          <HeaderCell>Service</HeaderCell>
          <HeaderCell>Span</HeaderCell>
          <HeaderCell className="text-right">Duration</HeaderCell>
          {spans ? <HeaderCell className="text-right">Spans</HeaderCell> : null}
        </tr>
      </thead>
      <tbody>
        {rows.map((row) => (
          <tr key={row.traceId}>
            <Cell>
              <Link
                {...traceLink(row.traceId, datasourceUid, range)}
                aria-label={`Open trace ${row.traceId}`}
                className="font-mono underline-offset-3 hover:underline"
              >
                {row.traceId.slice(0, 16)}
              </Link>
            </Cell>
            <Cell className="whitespace-nowrap tabular-nums">
              {row.start === null ? "-" : zonedDateTimeMs(row.start, undefined)}
            </Cell>
            <Cell>{row.service || "-"}</Cell>
            <Cell className="max-w-80 truncate">{row.span || "-"}</Cell>
            <Cell className="text-right whitespace-nowrap tabular-nums">
              {durationText(row.durationMs)}
            </Cell>
            {spans ? (
              <Cell className="text-right tabular-nums">
                {row.spans === null ? "-" : row.spans.toLocaleString("en")}
              </Cell>
            ) : null}
          </tr>
        ))}
      </tbody>
    </TableCard>
  )
}
