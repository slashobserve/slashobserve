import { useCallback, useState, type KeyboardEvent, type Ref } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { PageBar } from "@/components/page-bar"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { languageReference } from "@/datasources/sources"
import { defaultPageSize, pageStateOf, type PageSize } from "@/lib/paging"
import { traceLink } from "@/lib/links"
import { isModEnter, modChordLabel } from "@/lib/shortcuts"
import { lowerFirst } from "@/lib/text"
import { describeRange, exploreRange } from "@/lib/time"
import { exploreErrorText, exploreQuery, offlineText } from "@/logs/explore"
import { ExploreFailure } from "@/logs/explore-failure"
import { PlainTable } from "@/logs/plain-table"
import { labelNamesQuery } from "@/panel-editor/queries"
import { CodeEditor, type CodeEditorHandle } from "@/query-editor/code-editor"

import { AddToDashboardAction } from "./add-to-dashboard-action"
import { AddToObservationAction } from "./add-to-observation-action"
import { liveRefreshMs } from "./live-refresh"
import { rangeInFuture, type TraceSearch } from "./search"
import { traceSqlPanelFor } from "./trace-panel"
import {
  frameRows,
  readableCell,
  traceIdField,
  traceIdOf,
  traceSqlParams,
} from "./trace-sql"

const skeletonRows = [0, 1, 2, 3, 4]
const sqlReference = languageReference("logs", "clickhouse")
const linkedNote = "Each trace id opens its trace."
const unlinkedNote = "Select TraceId to open each trace from its row."

export function TraceSqlEditor({
  datasource,
  initial,
  onRun,
  editorRef,
}: {
  datasource: DatasourceStatus
  initial: string
  onRun: (query: string) => void
  editorRef: Ref<CodeEditorHandle>
}) {
  const [draft, setDraft] = useState(initial)
  const names = useQuery(
    labelNamesQuery(datasource.uid, { table: "otel_traces" })
  )
  const empty = draft.trim() === ""

  function run() {
    if (!empty) onRun(draft)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (!isModEnter(event) || event.nativeEvent.isComposing) return
    event.preventDefault()
    event.stopPropagation()
    run()
  }

  return (
    <>
      <div className="flex flex-col gap-1 px-4 pt-3 pb-4">
        <div onKeyDownCapture={onKeyDown} className="[&>div]:bg-background">
          <CodeEditor
            value={draft}
            onChange={setDraft}
            datasource={{ type: datasource.type, uid: datasource.uid }}
            readOnly={false}
            ariaLabel="SQL query"
            onLint={() => undefined}
            editorRef={editorRef}
            completionNames={names.data?.labels}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          Ctrl Space completes keywords and columns.{" "}
          {sqlReference === null ? null : (
            <a
              href={sqlReference.url}
              target="_blank"
              rel="noreferrer"
              className="underline underline-offset-3"
            >
              {sqlReference.label}
            </a>
          )}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-3 border-t px-4 py-2.5">
        <span className="text-xs text-muted-foreground">
          or press {modChordLabel(navigator.userAgent, "")}Enter
        </span>
        <Button
          type="button"
          size="sm"
          disabled={empty}
          title={empty ? "Write a query first." : "Run query"}
          aria-keyshortcuts="Control+Enter Meta+Enter"
          className="narrow:h-11"
          onClick={run}
        >
          Run query
        </Button>
      </div>
    </>
  )
}

function TraceSqlTable({
  frame,
  idField,
  datasourceUid,
  from,
  to,
  observation,
}: {
  frame: WireFrame
  idField: string | undefined
  datasourceUid: string
  from: string
  to: string
  observation: TraceSearch["observation"]
}) {
  const renderCell = useCallback(
    (field: string, value: unknown, type: string) => {
      const readable = readableCell(field, type, value)
      if (readable !== null) {
        return <span title={readable.raw}>{readable.text}</span>
      }
      if (field !== idField) return null
      const traceId = traceIdOf(value)
      if (traceId === null) return null
      return (
        <Link
          {...traceLink(
            traceId,
            datasourceUid,
            { from, to },
            undefined,
            observation
          )}
          className="underline underline-offset-3"
        >
          {traceId}
        </Link>
      )
    },
    [idField, datasourceUid, from, to, observation]
  )
  return (
    <PlainTable
      frame={frame}
      note={idField === undefined ? unlinkedNote : linkedNote}
      renderCell={renderCell}
    />
  )
}

export function TraceSqlResults({
  datasource,
  query,
  search,
  onPage,
  onPageSize,
  live,
}: {
  datasource: DatasourceStatus
  query: string
  search: TraceSearch
  onPage: (page: number) => void
  onPageSize: (size: PageSize | undefined) => void
  live: boolean
}) {
  const range = exploreRange(search)
  const [openedAt] = useState(Date.now)
  const future = rangeInFuture(range, openedAt)
  const page = search.page ?? 1
  const pageSize: PageSize = search.size ?? defaultPageSize
  const results = useQuery({
    ...exploreQuery(datasource.uid, traceSqlParams(query, search)),
    refetchInterval: liveRefreshMs(live, range),
    refetchIntervalInBackground: false,
    enabled: !future,
  })
  const data = results.data

  if (future) {
    return (
      <p role="status" className="text-[13px]">
        <StatusLabel tone="warning">
          This time range starts in the future, so no row can match. Pick an
          earlier range.
        </StatusLabel>
      </p>
    )
  }
  if (results.fetchStatus === "paused" && !data) {
    return <p className="text-[13px] text-muted-foreground">{offlineText}</p>
  }
  if (results.isError) {
    return (
      <ExploreFailure
        text={exploreErrorText(results.error)}
        onRetry={() => void results.refetch()}
      />
    )
  }
  if (!data) {
    return (
      <div aria-busy="true" className="flex flex-col gap-2">
        {skeletonRows.map((row) => (
          <Skeleton key={row} className="h-8" />
        ))}
      </div>
    )
  }
  const frame = data.frames.at(0)
  const rows = frameRows(frame)
  const state = pageStateOf(data.meta.page, rows, true, {
    page,
    pageSize,
    truncated: data.meta.truncated,
  })
  const pageBar = (
    <PageBar
      state={state}
      noun="rows"
      qualifier=" in range"
      label="Result pages"
      busy={results.isFetching}
      onPage={onPage}
      onPageSize={(size) =>
        onPageSize(size === defaultPageSize ? undefined : size)
      }
    />
  )
  if (frame === undefined || rows === 0) {
    return (
      <div className="flex flex-col gap-3">
        <p className="text-[13px] text-muted-foreground">
          No rows in {lowerFirst(describeRange(range.from, range.to))}.
        </p>
        {state.hasPrev ? pageBar : null}
      </div>
    )
  }
  const idField = traceIdField(frame)
  const panel = traceSqlPanelFor({
    id: "trace-sql",
    title: "Trace query",
    datasource: datasource.uid,
    query,
    y: 0,
  })
  return (
    <div className="flex flex-col gap-1">
      <div className="flex flex-col rounded-card border bg-card">
        <div className="flex flex-wrap items-center justify-end gap-2 border-b px-3 py-2">
          <AddToDashboardAction panel={panel} />
          <AddToObservationAction range={range} panel={panel} />
        </div>
        <TraceSqlTable
          frame={frame}
          idField={idField}
          datasourceUid={datasource.uid}
          from={range.from}
          to={range.to}
          observation={search.observation}
        />
      </div>
      {pageBar}
    </div>
  )
}
