import { useId, useMemo, useState, type ReactNode } from "react"
import { useQuery } from "@tanstack/react-query"
import { Link } from "@tanstack/react-router"

import { Alert, AlertDescription } from "@/components/ui/alert"
import { Button, buttonVariants } from "@/components/ui/button"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Skeleton } from "@/components/ui/skeleton"
import type { DatasourceStatus } from "@/datasources/queries"
import { logsLink } from "@/lib/links"
import { cn } from "@/lib/utils"
import { exploreErrorText } from "@/logs/explore"
import { ExploreLogs } from "@/logs/logs-results"
import { useElementSize } from "@/logs/use-element-size"
import type { Span } from "@/panels/traces/trace-model"

import {
  lineCountText,
  logsDatasourceCandidates,
  logsDatasourceFor,
  matchNote,
  previewTime,
  rememberLogsDatasource,
  traceLogLines,
  traceLogsQuery,
  traceLogsSearch,
  traceLogsWindow,
  truncationNote,
} from "./trace-logs"

type Props = {
  traceUid: string
  traceId: string
  span?: Span
  trace: { start: number; duration: number }
  all: readonly DatasourceStatus[]
  preview: boolean
  onOpenTraceLogs?: () => void
}

const previewLines = 3
const previewSkeleton = [0, 1, 2]
const panelSkeleton = [0, 1, 2, 3, 4]

const panelDisplay = {
  showTime: true,
  showLabels: false,
  details: true,
  detailsMode: "inline",
  controls: false,
  order: "oldest",
}

function Heading({ title, count }: { title: string; count: string | null }) {
  return (
    <h3 className="m-0 text-xs font-semibold text-foreground">
      {title}
      {count === null ? null : (
        <span className="font-normal text-muted-foreground"> {count}</span>
      )}
    </h3>
  )
}

function DatasourceChoice({
  candidates,
  value,
  onChange,
}: {
  candidates: readonly DatasourceStatus[]
  value: string
  onChange: (uid: string) => void
}) {
  const id = useId()
  const items = candidates.map((candidate) => ({
    value: candidate.uid,
    label: candidate.name,
  }))
  return (
    <div className="flex items-center gap-2">
      <label htmlFor={id} className="text-xs text-muted-foreground">
        Logs datasource
      </label>
      <Select
        items={items}
        value={value}
        onValueChange={(next) => {
          if (next !== null) onChange(next)
        }}
      >
        <SelectTrigger id={id} size="sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false} align="start">
          {items.map((item) => (
            <SelectItem key={item.value} value={item.value}>
              {item.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

export function TraceLogsSection({
  traceUid,
  traceId,
  span,
  trace,
  all,
  preview,
  onOpenTraceLogs,
}: Props) {
  const [pickedUid, setPickedUid] = useState<string | null>(null)
  const candidates = logsDatasourceCandidates(all)
  const datasource =
    candidates.find((candidate) => candidate.uid === pickedUid) ??
    logsDatasourceFor(traceUid, all)
  const subject = span ? "span" : "trace"
  const title = `Logs for this ${subject}`

  if (datasource === null) {
    return (
      <section className="flex flex-col gap-1.5 text-xs">
        <Heading title={title} count={null} />
        <p className="text-muted-foreground">
          Connect a Loki or ClickHouse logs datasource to see lines.{" "}
          <Link to="/datasources" className="text-foreground underline">
            Datasources
          </Link>
        </p>
      </section>
    )
  }

  const choice =
    candidates.length > 1 ? (
      <DatasourceChoice
        candidates={candidates}
        value={datasource.uid}
        onChange={(uid) => {
          rememberLogsDatasource(uid)
          setPickedUid(uid)
        }}
      />
    ) : null

  return (
    <TraceLogsBody
      datasource={datasource}
      traceId={traceId}
      span={span}
      trace={trace}
      preview={preview}
      title={title}
      subject={subject}
      choice={choice}
      onOpenTraceLogs={onOpenTraceLogs}
    />
  )
}

function TraceLogsBody({
  datasource,
  traceId,
  span,
  trace,
  preview,
  title,
  subject,
  choice,
  onOpenTraceLogs,
}: {
  datasource: DatasourceStatus
  traceId: string
  span: Span | undefined
  trace: { start: number; duration: number }
  preview: boolean
  title: string
  subject: string
  choice: ReactNode
  onOpenTraceLogs: (() => void) | undefined
}) {
  const window = span
    ? traceLogsWindow(span.start, span.duration)
    : traceLogsWindow(trace.start, trace.duration)
  const result = useQuery(
    traceLogsQuery(datasource.uid, {
      traceId,
      spanId: span?.id,
      service: span?.service,
      ...window,
    })
  )
  const frames = useMemo(() => result.data?.frames ?? [], [result.data])
  const lines = useMemo(() => traceLogLines(frames), [frames])
  const truncated = result.data?.meta.truncated === true
  const count = result.data ? lineCountText(lines.length, truncated) : null
  const { ref, height } = useElementSize<HTMLDivElement>()
  const open = (
    <Link
      {...logsLink(
        datasource.uid,
        traceLogsSearch(datasource.type, traceId, window, span?.id)
      )}
      className={cn(buttonVariants({ variant: "outline", size: "xs" }))}
    >
      Open in Logs
    </Link>
  )

  function body() {
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
    if (result.isPending) {
      return (
        <div aria-busy="true" className="flex flex-col gap-1.5">
          {(preview ? previewSkeleton : panelSkeleton).map((row) => (
            <Skeleton key={row} className="h-5" />
          ))}
        </div>
      )
    }
    if (lines.length === 0) {
      return (
        <div className="flex flex-wrap items-center gap-2">
          <p className="m-0 text-muted-foreground">
            No log lines for this {subject}.
          </p>
          {preview && onOpenTraceLogs ? (
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={onOpenTraceLogs}
            >
              Try the whole trace
            </Button>
          ) : null}
        </div>
      )
    }
    if (preview) {
      return (
        <ol className="m-0 flex list-none flex-col gap-1 p-0">
          {lines.slice(0, previewLines).map((line, index) => (
            <li
              key={index}
              className="grid grid-cols-[auto_minmax(0,1fr)] gap-2 text-xs"
            >
              <span className="font-mono text-muted-foreground tabular-nums">
                {previewTime(line.time)}
              </span>
              <span className="truncate" title={line.body}>
                {line.body}
              </span>
            </li>
          ))}
        </ol>
      )
    }
    return height > 0 ? (
      <ExploreLogs
        frames={frames}
        datasourceUid={datasource.uid}
        title={title}
        height={height}
        display={panelDisplay}
      />
    ) : null
  }

  return (
    <section
      className={cn(
        "flex flex-col gap-1.5 text-xs",
        !preview && "min-h-0 flex-1"
      )}
    >
      <Heading title={title} count={count} />
      {choice}
      {preview ? (
        body()
      ) : (
        <div ref={ref} className="relative min-h-80 flex-1">
          <div className="absolute inset-0 flex flex-col gap-2">{body()}</div>
        </div>
      )}
      {!preview && truncated ? (
        <p className="m-0 text-muted-foreground">{truncationNote}</p>
      ) : null}
      {result.data && lines.length > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          {open}
          <span className="text-muted-foreground">
            {matchNote(datasource.type, span)}
          </span>
        </div>
      ) : null}
    </section>
  )
}
