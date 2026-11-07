import { useMemo, useState } from "react"
import { useQuery } from "@tanstack/react-query"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import type { DatasourceStatus } from "@/datasources/queries"
import type { LogSelection } from "@/panels/logs/logs-panel"

import {
  anchorFrames,
  anchorMatcher,
  anchorNs,
  contextFrames,
  contextQuery,
  contextQueryFor,
  contextStep,
  contextWindows,
  dateFieldNote,
  defaultContextLines,
  defaultContextWindowMs,
  maxContextLines,
  moreLines,
  type LogContext,
  serviceNameOf,
} from "./context-query"
import { exploreErrorText } from "./explore"
import { rowCount } from "./frames"
import { ExploreLogs } from "./logs-results"
import { isSearchFamily } from "./search"
import { useElementSize } from "./use-element-size"

type Props = {
  datasource: DatasourceStatus
  anchor: LogSelection
  indexedLabels: readonly string[] | undefined
}

const contextDisplay = {
  showTime: true,
  showLabels: false,
  details: true,
  detailsMode: "inline",
  controls: false,
  order: "oldest",
}

const skeletonRows = [0, 1, 2, 3, 4]

const windowItems = contextWindows.map(({ ms, label }) => ({
  value: String(ms),
  label,
}))

const dateFieldText = "On a date field, context is exact to the millisecond."

export function ContextTab(props: Props) {
  const { entry } = props.anchor
  return (
    <ContextLines
      key={`${props.datasource.uid}|${anchorNs(entry)}|${entry.message}`}
      {...props}
    />
  )
}

function ContextLines({ datasource, anchor, indexedLabels }: Props) {
  const { entry } = anchor
  const uid = datasource.uid
  const searchFamily = isSearchFamily(datasource.type)
  const [before, setBefore] = useState(defaultContextLines)
  const [after, setAfter] = useState(defaultContextLines)
  const [windowMs, setWindowMs] = useState(defaultContextWindowMs)
  const query = contextQueryFor(
    datasource.type,
    entry,
    anchor.streamLabels,
    indexedLabels
  )
  const ns = anchorNs(entry)
  const unavailable =
    query === null
      ? "This line has no stream labels to search by."
      : ns === null
        ? "This line has no time."
        : null

  const context = useQuery({
    ...contextQuery(uid, {
      query: query ?? "",
      anchor: ns ?? "",
      before,
      after,
      window_ms: windowMs,
    }),
    enabled: unavailable === null,
  })

  const { data } = context
  const frames = useMemo(
    () => (data ? contextFrames(data, anchorFrames(entry)) : []),
    [data, entry]
  )
  const isMarked = useMemo(() => anchorMatcher(entry), [entry])
  const { ref, height } = useElementSize<HTMLDivElement>()
  const windowLabel =
    contextWindows.find((choice) => choice.ms === windowMs)?.label ?? ""
  const service = serviceNameOf(entry)
  const scope =
    datasource.type === "loki"
      ? query
      : datasource.type === "clickhouse" && service !== null
        ? `ServiceName = ${service}`
        : null

  function body() {
    if (unavailable !== null) {
      return <p className="text-[13px] text-muted-foreground">{unavailable}</p>
    }
    if (context.error) {
      return (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-col items-start gap-2">
            {exploreErrorText(context.error)}
            <Button
              type="button"
              variant="outline"
              size="xs"
              onClick={() => void context.refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )
    }
    if (!data) {
      return (
        <div aria-busy="true" className="flex flex-col gap-2">
          {skeletonRows.map((row) => (
            <Skeleton key={row} className="h-5" />
          ))}
        </div>
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

  function loadMore(
    label: string,
    count: number,
    truncated: boolean | undefined,
    set: (next: number) => void
  ) {
    if (!truncated || count >= maxContextLines) return null
    return (
      <Button
        type="button"
        variant="outline"
        size="xs"
        className="self-start"
        disabled={context.isPlaceholderData}
        onClick={() => set(moreLines(count))}
      >
        {label}
      </Button>
    )
  }

  function emptyNote(side: LogContext["before"], direction: string) {
    if (side.truncated || rowCount(side.frames) > 0) return null
    return (
      <p className="text-[13px] text-muted-foreground">
        No more lines within {windowLabel} {direction}.
      </p>
    )
  }

  return (
    <section
      aria-label={searchFamily ? "Surrounding documents" : "Surrounding lines"}
      className="flex min-h-0 flex-1 flex-col gap-2"
    >
      <h3 className="text-[13px] font-medium">
        {searchFamily ? "Surrounding documents" : "Surrounding lines"}
      </h3>
      {scope !== null ? (
        <p className="font-mono text-xs [overflow-wrap:anywhere]">{scope}</p>
      ) : null}
      {loadMore(
        `Load ${contextStep} older`,
        before,
        data?.before.truncated,
        setBefore
      )}
      <div ref={ref} className="relative min-h-80 flex-1">
        <div className="absolute inset-0 flex flex-col gap-2">{body()}</div>
      </div>
      {loadMore(
        `Load ${contextStep} newer`,
        after,
        data?.after.truncated,
        setAfter
      )}
      {data ? (
        <>
          {emptyNote(data.before, "before")}
          {emptyNote(data.after, "after")}
          {dateFieldNote(datasource.type, frames) ? (
            <p className="text-[13px] text-muted-foreground">{dateFieldText}</p>
          ) : null}
        </>
      ) : null}
      {unavailable === null ? (
        <div className="flex items-center gap-2">
          <span className="text-[13px] text-muted-foreground">Window</span>
          <Select
            items={windowItems}
            value={String(windowMs)}
            onValueChange={(next) => {
              if (next !== null) setWindowMs(Number(next))
            }}
          >
            <SelectTrigger size="sm" aria-label="Window">
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              {windowItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ) : null}
    </section>
  )
}
