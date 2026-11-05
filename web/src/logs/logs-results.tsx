import { useCallback, useMemo } from "react"
import { Link } from "@tanstack/react-router"

import { Button, buttonVariants } from "@/components/ui/button"
import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { traceLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { LogsView } from "@/panels/logs/logs-panel"
import type { Labels, LogEntry, LogsDisplay } from "@/panels/logs/logs-model"
import type { DataPanel } from "@/spec"

import { serviceNameOf } from "./context-query"
import { usePrepared } from "./prepared"
import { traceIdOf, traceRangeAround } from "./trace-link"
import { useElementSize } from "./use-element-size"

type ExploreLogsProps = {
  frames: WireFrame[]
  datasourceUid: string
  title: string
  height: number
  display: Record<string, unknown>
  detailActions?: (entry: LogEntry, streamLabels: Labels) => React.ReactNode
  isMarked?: (entry: LogEntry) => boolean
}

export function ExploreLogs({
  frames,
  datasourceUid,
  title,
  height,
  display,
  detailActions,
  isMarked,
}: ExploreLogsProps) {
  const panel = useMemo<DataPanel>(
    () => ({
      id: "logs",
      type: "logs",
      title: "Logs",
      datasource: datasourceUid,
      gridPos: { x: 0, y: 0, w: 24, h: 10 },
      queries: [{ query: "" }],
      display,
    }),
    [datasourceUid, display]
  )
  const prepared = usePrepared(panel, frames)
  const { ref, width } = useElementSize<HTMLDivElement>()
  return (
    <div ref={ref} className="min-w-0" style={{ height }}>
      {width > 0 ? (
        <LogsView
          panelTitle={title}
          frames={prepared.frames}
          fields={prepared.fields}
          zone={undefined}
          display={prepared.display as LogsDisplay}
          width={width}
          height={height}
          detailActions={detailActions}
          isMarked={isMarked}
        />
      ) : null}
    </div>
  )
}

const resultsDisplay = {
  showTime: true,
  showLabels: true,
  details: true,
  detailsMode: "sidebar",
  controls: true,
}

export type ContextRequest = { entry: LogEntry; streamLabels: Labels }

export function LogsResults({
  frames,
  datasource,
  onContext,
  traceDatasource,
  height,
}: {
  frames: WireFrame[]
  datasource: DatasourceStatus
  onContext: (request: ContextRequest) => void
  traceDatasource: string | null
  height: number
}) {
  const clickhouse = datasource.type === "clickhouse"
  const detailActions = useCallback(
    (entry: LogEntry, streamLabels: Labels) => {
      const traceId = traceIdOf(entry)
      const noContext =
        entry.time === null
          ? "This line has no time."
          : clickhouse && serviceNameOf(entry) === null
            ? "This line has no ServiceName."
            : undefined
      return (
        <>
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={noContext !== undefined}
            title={noContext}
            onClick={() => onContext({ entry, streamLabels })}
          >
            Show context
          </Button>
          {traceId !== null &&
          traceDatasource !== null &&
          entry.time !== null ? (
            <Link
              {...traceLink(
                traceId,
                traceDatasource,
                traceRangeAround(entry.time)
              )}
              aria-label={`Open trace ${traceId}`}
              className={cn(buttonVariants({ variant: "outline", size: "xs" }))}
            >
              Open trace
            </Link>
          ) : null}
        </>
      )
    },
    [clickhouse, onContext, traceDatasource]
  )
  return (
    <ExploreLogs
      frames={frames}
      datasourceUid={datasource.uid}
      title="Query results"
      height={height}
      display={resultsDisplay}
      detailActions={detailActions}
    />
  )
}
