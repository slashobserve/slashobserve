import { useMemo } from "react"
import { useQuery } from "@tanstack/react-query"

import { exploreKeys } from "@/api/keys"
import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import type { LogSelection } from "@/panels/logs/logs-panel"
import type { LogEntry } from "@/panels/logs/logs-model"

import {
  anchorRowKey,
  clickhouseContextQueries,
  contextFrames,
  lokiAfterQuery,
  lokiBeforeQuery,
  lokiStreamSelector,
  nextAfterStep,
  rowsPerAfterStep,
  serviceNameOf,
} from "./context-query"
import { exploreErrorText, exploreRequest } from "./explore"
import { rowCount, rowTime } from "./frames"
import { ExploreLogs } from "./logs-results"
import { useElementSize } from "./use-element-size"

type Props = {
  datasource: DatasourceStatus
  anchor: LogSelection
  indexedLabels: readonly string[] | undefined
}

type AfterLines = { frames: WireFrame[]; incomplete: boolean }

const contextDisplay = {
  showTime: true,
  showLabels: false,
  details: true,
  detailsMode: "inline",
  controls: false,
  order: "oldest",
}

const skeletonRows = [0, 1, 2, 3, 4]

function linesAfter(frames: readonly WireFrame[], anchorMs: number): number {
  let count = 0
  for (const frame of frames) {
    const rows = Math.max(
      0,
      ...frame.fields.map((field) => field.values.length)
    )
    for (let row = 0; row < rows; row++) {
      const time = rowTime(frame, row)
      if (time !== null && time > anchorMs) count++
    }
  }
  return count
}

async function lokiAfterLines(
  uid: string,
  selector: string,
  anchorMs: number,
  signal: AbortSignal
): Promise<AfterLines> {
  let step: number | null = 0
  let result: AfterLines = { frames: [], incomplete: false }
  while (step !== null) {
    const data = await exploreRequest(
      uid,
      lokiAfterQuery(selector, anchorMs, step),
      signal
    )
    const saturated = rowCount(data.frames) === rowsPerAfterStep
    result = {
      frames: data.frames,
      incomplete: result.incomplete || (step === 0 && saturated),
    }
    step = nextAfterStep(step, linesAfter(data.frames, anchorMs), saturated)
  }
  return result
}

type Search =
  | { kind: "loki"; selector: string }
  | { kind: "clickhouse"; service: string }
  | { kind: "none"; reason: string }

function searchFor(
  datasource: DatasourceStatus,
  anchor: LogSelection,
  indexedLabels: readonly string[] | undefined
): Search {
  if (datasource.type === "clickhouse") {
    const service = serviceNameOf(anchor.entry)
    return service === null
      ? { kind: "none", reason: "This line has no ServiceName to search by." }
      : { kind: "clickhouse", service }
  }
  const selector = lokiStreamSelector(anchor.streamLabels, indexedLabels)
  return selector === null
    ? { kind: "none", reason: "This line has no stream labels to search by." }
    : { kind: "loki", selector }
}

export function ContextTab({ datasource, anchor, indexedLabels }: Props) {
  const { entry } = anchor
  const anchorMs = entry.time ?? 0
  const rowKey = anchorRowKey(entry)
  const anchorKey = rowKey ?? `${anchorMs}|${entry.message}`
  const search = searchFor(datasource, anchor, indexedLabels)
  const uid = datasource.uid

  const before = useQuery({
    queryKey: exploreKeys.context(uid, `${anchorKey}:before`),
    enabled: search.kind !== "none",
    queryFn: ({ signal }) =>
      exploreRequest(
        uid,
        search.kind === "loki"
          ? lokiBeforeQuery(search.selector, anchorMs)
          : clickhouseContextQueries(
              search.kind === "clickhouse" ? search.service : "",
              anchorMs
            ).before,
        signal
      ),
    retry: false,
    staleTime: 60_000,
  })
  const after = useQuery({
    queryKey: exploreKeys.context(uid, `${anchorKey}:after`),
    enabled: search.kind !== "none",
    queryFn: async ({ signal }): Promise<AfterLines> => {
      if (search.kind === "loki") {
        return lokiAfterLines(uid, search.selector, anchorMs, signal)
      }
      const data = await exploreRequest(
        uid,
        clickhouseContextQueries(
          search.kind === "clickhouse" ? search.service : "",
          anchorMs
        ).after,
        signal
      )
      return { frames: data.frames, incomplete: false }
    },
    retry: false,
    staleTime: 60_000,
  })

  const frames = useMemo(
    () =>
      before.data && after.data
        ? contextFrames([...before.data.frames, ...after.data.frames], {
            time: anchorMs,
            key: rowKey,
          })
        : [],
    [before.data, after.data, anchorMs, rowKey]
  )
  const isMarked = useMemo(
    () => (candidate: LogEntry) =>
      candidate.time === entry.time && candidate.message === entry.message,
    [entry.time, entry.message]
  )
  const { ref, height } = useElementSize<HTMLDivElement>()
  const error = before.error ?? after.error
  const loading = before.isPending || after.isPending

  function body() {
    if (search.kind === "none") {
      return (
        <p className="text-[13px] text-muted-foreground">{search.reason}</p>
      )
    }
    if (error) {
      return (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {exploreErrorText(error)}
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => {
                if (before.isError) void before.refetch()
                if (after.isError) void after.refetch()
              }}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )
    }
    if (loading) {
      return (
        <div aria-busy="true" className="flex flex-col gap-2">
          {skeletonRows.map((row) => (
            <Skeleton key={row} className="h-5" />
          ))}
        </div>
      )
    }
    if (rowCount(frames) <= 1) {
      return (
        <p className="text-[13px] text-muted-foreground">
          No other lines in this stream within an hour.
        </p>
      )
    }
    return height > 0 ? (
      <ExploreLogs
        frames={frames}
        datasourceUid={uid}
        title="Lines around this line"
        height={height}
        display={contextDisplay}
        isMarked={isMarked}
      />
    ) : null
  }

  return (
    <>
      {search.kind !== "none" ? (
        <p className="font-mono text-xs [overflow-wrap:anywhere]">
          {search.kind === "loki"
            ? search.selector
            : `ServiceName = ${search.service}`}
        </p>
      ) : null}
      {after.data?.incomplete ? (
        <p className="text-[13px] text-muted-foreground">
          More than 1,000 lines arrived in the 10 seconds after this line, so
          the lines after it are not complete.
        </p>
      ) : null}
      <div ref={ref} className="relative min-h-80 flex-1">
        <div className="absolute inset-0 flex flex-col gap-2">{body()}</div>
      </div>
    </>
  )
}
