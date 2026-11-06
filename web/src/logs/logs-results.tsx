import { useCallback, useMemo, useSyncExternalStore } from "react"
import { Link } from "@tanstack/react-router"

import { CopyButton } from "@/components/copy-button"
import { Button, buttonVariants } from "@/components/ui/button"
import type { WireFrame } from "@/dashboards/queries"
import type { DatasourceStatus } from "@/datasources/queries"
import { traceLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import type { LogEntry, LogsDisplay } from "@/panels/logs/logs-model"
import { LogsView, type LogSelection } from "@/panels/logs/logs-panel"
import type { DataPanel } from "@/spec"

import { serviceNameOf } from "./context-query"
import { usePrepared } from "./prepared"
import { isSearchFamily } from "./search"
import { traceIdOf, traceRangeAround } from "./trace-link"
import { useElementSize } from "./use-element-size"

type ExploreLogsProps = {
  frames: WireFrame[]
  datasourceUid: string
  title: string
  height: number
  display: Record<string, unknown>
  detailActions?: (selection: LogSelection) => React.ReactNode
  isMarked?: (entry: LogEntry) => boolean
  selectedKey?: string | null
  onSelectedChange?: (selection: LogSelection | null) => void
  detailsId?: string
}

export function ExploreLogs({
  frames,
  datasourceUid,
  title,
  height,
  display,
  detailActions,
  isMarked,
  selectedKey,
  onSelectedChange,
  detailsId,
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
          selectedKey={selectedKey}
          onSelectedChange={onSelectedChange}
          detailsId={detailsId}
        />
      ) : null}
    </div>
  )
}

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

const resultsDisplay = {
  showTime: true,
  showLabels: true,
  details: true,
  detailsMode: "inline",
  controls: true,
}

export function contextBlocker(
  datasource: DatasourceStatus,
  entry: LogEntry
): string | undefined {
  if (isSearchFamily(datasource.type)) {
    return "Surrounding documents are not available on this datasource type yet."
  }
  if (entry.time === null) return "This line has no time."
  if (datasource.type === "clickhouse" && serviceNameOf(entry) === null) {
    return "This line has no ServiceName."
  }
  return undefined
}

export function OpenTraceLink({
  traceId,
  traceDatasource,
  time,
  className,
  children,
}: {
  traceId: string
  traceDatasource: string
  time: number
  className?: string
  children: React.ReactNode
}) {
  return (
    <Link
      {...traceLink(traceId, traceDatasource, traceRangeAround(time))}
      aria-label={`Open trace ${traceId}`}
      className={className}
    >
      {children}
    </Link>
  )
}

export function LogsResults({
  frames,
  datasource,
  traceDatasource,
  height,
  selectedKey,
  onSelect,
  onContext,
  detailsId,
}: {
  frames: WireFrame[]
  datasource: DatasourceStatus
  traceDatasource: string | null
  height: number
  selectedKey: string | null
  onSelect: (selection: LogSelection | null) => void
  onContext: (selection: LogSelection) => void
  detailsId: string
}) {
  const phone = usePhone()
  const detailActions = useCallback(
    (selection: LogSelection) => {
      const { entry } = selection
      const traceId = traceIdOf(entry)
      const noContext = contextBlocker(datasource, entry)
      return (
        <>
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={noContext !== undefined}
            title={noContext}
            onClick={() => onContext(selection)}
          >
            Show context
          </Button>
          {traceId !== null &&
          traceDatasource !== null &&
          entry.time !== null ? (
            <OpenTraceLink
              traceId={traceId}
              traceDatasource={traceDatasource}
              time={entry.time}
              className={cn(buttonVariants({ variant: "outline", size: "xs" }))}
            >
              Open trace
            </OpenTraceLink>
          ) : null}
          <CopyButton
            text={entry.message}
            label="Copy line"
            variant="outline"
            size="xs"
            icon={false}
          >
            Copy line
          </CopyButton>
        </>
      )
    },
    [datasource, onContext, traceDatasource]
  )
  return (
    <ExploreLogs
      frames={frames}
      datasourceUid={datasource.uid}
      title="Query results"
      height={height}
      display={resultsDisplay}
      detailActions={phone ? detailActions : undefined}
      selectedKey={selectedKey}
      onSelectedChange={phone ? undefined : onSelect}
      detailsId={detailsId}
    />
  )
}
