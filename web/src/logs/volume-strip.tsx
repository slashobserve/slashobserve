import {
  useId,
  useMemo,
  useState,
  type KeyboardEvent,
  type PointerEvent,
} from "react"
import { useQuery } from "@tanstack/react-query"

import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { zonedDateTime } from "@/lib/time"
import { cn } from "@/lib/utils"
import type { LogLevelKind } from "@/panels/logs/logs-model"

import {
  brushRange,
  bucketAt,
  bucketSpans,
  moveEdge,
  selectionOf,
  walkBucket,
  type BucketSelection,
} from "./brush"
import { exploreErrorText } from "./explore"
import {
  bucketTotal,
  logVolumeQuery,
  selectionEdges,
  selectionText,
  volumeBucketText,
  volumeCaption,
  volumeFailureText,
  volumeKindNames,
  volumeKinds,
  volumeLabel,
  volumeTotals,
  type LogVolume,
} from "./volume"
import { useLogsWorkspace } from "./workspace"

const kindClasses: Record<LogLevelKind, string> = {
  error: "bg-status-critical",
  warning: "bg-status-warning",
  info: "bg-series-1",
  other: "bg-muted-foreground/40",
}

const chipClasses =
  "text-muted-foreground aria-pressed:bg-muted aria-pressed:text-foreground"

function count(value: number): string {
  return value.toLocaleString("en")
}

function KindSquare({ kind }: { kind: LogLevelKind }) {
  return (
    <span
      aria-hidden="true"
      className={cn("size-2 rounded-chip", kindClasses[kind])}
    />
  )
}

export function VolumeStrip({
  uid,
  query,
  from,
  to,
}: {
  uid: string
  query: string
  from: string
  to: string
}) {
  const { update } = useLogsWorkspace()
  const volume = useQuery(logVolumeQuery(uid, { query, from, to }))
  if (volume.isError) {
    return (
      <div className="flex flex-col gap-1">
        <div className="flex items-center justify-between gap-3 text-xs leading-4 text-muted-foreground">
          <p>Counts arrive with the volume</p>
          <div
            role="group"
            aria-label="Levels"
            className="flex items-center gap-1"
          >
            <Button
              type="button"
              variant="outline"
              size="xs"
              disabled
              className={chipClasses}
            >
              All
            </Button>
            {volumeKinds.map((kind) => (
              <Button
                key={kind}
                type="button"
                variant="outline"
                size="xs"
                disabled
                className={chipClasses}
              >
                <KindSquare kind={kind} />
                {volumeKindNames[kind]}
              </Button>
            ))}
          </div>
        </div>
        <div
          aria-busy={volume.isFetching}
          className="flex h-12 items-center gap-2 text-[13px]"
        >
          <p className="min-w-0 truncate">
            {volumeFailureText(exploreErrorText(volume.error))}
          </p>
          <Button
            type="button"
            variant="outline"
            size="xs"
            disabled={volume.isFetching}
            onClick={() => void volume.refetch()}
          >
            Retry volume
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={() => update({ from: "now-15m", to: "now" })}
          >
            Try the last 15 minutes
          </Button>
        </div>
      </div>
    )
  }
  if (volume.data === undefined) {
    return <Skeleton aria-hidden="true" className="h-12 rounded-control" />
  }
  return (
    <VolumeBars
      key={volume.dataUpdatedAt}
      volume={volume.data}
      from={from}
      to={to}
      onZoom={(range) => update(range)}
    />
  )
}

function VolumeBars({
  volume,
  from,
  to,
  onZoom,
}: {
  volume: LogVolume
  from: string
  to: string
  onZoom: (range: { from: string; to: string }) => void
}) {
  const { buckets, total } = volume
  const bucketCount = buckets.length
  const describedBy = useId()
  const brushable = !useCoarsePointer()
  const [hidden, setHidden] = useState<ReadonlySet<LogLevelKind>>(new Set())
  const [hover, setHover] = useState<number | null>(null)
  const [drag, setDrag] = useState<{ anchor: number; to: number } | null>(null)
  const [selection, setSelection] = useState<BucketSelection | null>(null)
  const [extendFrom, setExtendFrom] = useState<number | null>(null)

  const totals = useMemo(() => volumeTotals(buckets), [buckets])
  const spans = useMemo(
    () =>
      bucketSpans(
        buckets.map((bucket) => Date.parse(bucket.start)),
        volume.step_ms
      ),
    [buckets, volume.step_ms]
  )
  const present = volumeKinds.filter((kind) => totals[kind] > 0)
  const shown = volumeKinds.filter((kind) => !hidden.has(kind))
  const highest = Math.max(1, ...buckets.map((b) => bucketTotal(b, shown)))
  const preview = drag
    ? {
        from: Math.min(drag.anchor, drag.to),
        to: Math.max(drag.anchor, drag.to),
      }
    : selection

  function toggle(kind: LogLevelKind) {
    setHidden((current) => {
      const next = new Set(current)
      if (!next.delete(kind)) next.add(kind)
      return next
    })
  }

  function bucketUnder(event: PointerEvent<HTMLDivElement>): number {
    const rect = event.currentTarget.getBoundingClientRect()
    return bucketAt(event.clientX - rect.left, rect.width, bucketCount)
  }

  function release(event: PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (!brushable || event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    const index = bucketUnder(event)
    setDrag({ anchor: index, to: index })
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const index = bucketUnder(event)
    setHover(index)
    setDrag((current) => (current ? { ...current, to: index } : current))
  }

  function onPointerUp(event: PointerEvent<HTMLDivElement>) {
    release(event)
    if (drag === null) return
    setSelection(selectionOf(drag.anchor, drag.to))
    setExtendFrom(null)
    setDrag(null)
  }

  function onPointerCancel(event: PointerEvent<HTMLDivElement>) {
    release(event)
    setDrag(null)
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "Escape") {
      if (drag === null && selection === null) return
      event.preventDefault()
      setDrag(null)
      setSelection(null)
      return
    }
    const next = walkBucket(event.key, hover, bucketCount)
    if (next === null) return
    event.preventDefault()
    setHover(next)
    if (!event.shiftKey || !brushable) {
      setExtendFrom(null)
      return
    }
    const anchor = extendFrom ?? hover ?? next
    setExtendFrom(anchor)
    setSelection(selectionOf(anchor, next))
  }

  function onHandleKeyDown(
    event: KeyboardEvent<HTMLButtonElement>,
    edge: "from" | "to"
  ) {
    if (selection === null) return
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
    event.preventDefault()
    setSelection(
      moveEdge(selection, edge, event.key === "ArrowLeft" ? -1 : 1, bucketCount)
    )
  }

  function zoom() {
    if (selection === null) return
    const range = brushRange(spans, selection.from, selection.to)
    if (range === null) return
    setSelection(null)
    onZoom(range)
  }

  const hovered = hover === null ? undefined : buckets[hover]
  const edges =
    selection === null
      ? null
      : selectionEdges(volume, selection.from, selection.to, undefined)
  const ratio = hover === null ? 0 : (hover + 0.5) / bucketCount
  const label = volumeLabel(total, totals)

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between gap-3 text-xs leading-4 text-muted-foreground">
        <p>{volumeCaption(total, from, to)}</p>
        <div
          role="group"
          aria-label="Levels"
          className="flex items-center gap-1"
        >
          <Button
            type="button"
            variant="outline"
            size="xs"
            aria-pressed={hidden.size === 0}
            className={chipClasses}
            onClick={() => setHidden(new Set())}
          >
            All
            <span className="tabular-nums">{count(total)}</span>
          </Button>
          {present.map((kind) => (
            <Button
              key={kind}
              type="button"
              variant="outline"
              size="xs"
              aria-pressed={!hidden.has(kind)}
              className={chipClasses}
              onClick={() => toggle(kind)}
            >
              <KindSquare kind={kind} />
              {volumeKindNames[kind]}
              <span className="tabular-nums">{count(totals[kind])}</span>
            </Button>
          ))}
        </div>
      </div>
      <div className="relative h-12">
        <div
          role="img"
          tabIndex={0}
          aria-label={brushable ? `${label}, drag to select a range.` : label}
          aria-describedby={describedBy}
          className={cn(
            "flex h-full items-end gap-px rounded-control select-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
            brushable && "cursor-col-resize touch-none"
          )}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
          onPointerLeave={() => setHover(null)}
          onBlur={() => setHover(null)}
          onKeyDown={onKeyDown}
        >
          {buckets.map((bucket) => {
            const lines = bucketTotal(bucket, shown)
            return (
              <div
                key={bucket.start}
                className="flex min-w-0 flex-1 flex-col-reverse"
                style={{ height: `${(lines / highest) * 100}%` }}
              >
                {shown.map((kind) =>
                  bucket[kind] > 0 ? (
                    <div
                      key={kind}
                      className={cn("min-h-px basis-0", kindClasses[kind])}
                      style={{ flexGrow: bucket[kind] }}
                    />
                  ) : null
                )}
              </div>
            )
          })}
        </div>
        {preview === null ? null : (
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 border-x border-ring bg-selection/40"
            style={{
              left: `${(preview.from / bucketCount) * 100}%`,
              width: `${((preview.to - preview.from + 1) / bucketCount) * 100}%`,
            }}
          />
        )}
        {selection === null || edges === null || drag !== null ? null : (
          <div role="group" aria-label="Selection handles">
            <button
              type="button"
              aria-label={`Selection start, ${edges.start}. Use arrow keys to move`}
              className="absolute inset-y-0 w-1 -translate-x-1/2 cursor-col-resize rounded-chip bg-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:outline-none"
              style={{ left: `${(selection.from / bucketCount) * 100}%` }}
              onKeyDown={(event) => onHandleKeyDown(event, "from")}
            />
            <button
              type="button"
              aria-label={`Selection end, ${edges.end}. Use arrow keys to move`}
              className="absolute inset-y-0 w-1 -translate-x-1/2 cursor-col-resize rounded-chip bg-ring focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:outline-none"
              style={{ left: `${((selection.to + 1) / bucketCount) * 100}%` }}
              onKeyDown={(event) => onHandleKeyDown(event, "to")}
            />
          </div>
        )}
        {hovered === undefined ? null : (
          <div
            aria-hidden="true"
            className={cn(
              "pointer-events-none absolute bottom-full z-10 mb-1 w-max max-w-72 rounded-card border border-border bg-popover p-2 text-xs text-popover-foreground",
              ratio < 0.15
                ? "translate-x-0"
                : ratio > 0.85
                  ? "-translate-x-full"
                  : "-translate-x-1/2"
            )}
            style={{ left: `${ratio * 100}%` }}
          >
            <p className="text-muted-foreground tabular-nums">
              {zonedDateTime(hovered.start, undefined)}
            </p>
            <p className="tabular-nums">{volumeBucketText(hovered)}</p>
          </div>
        )}
        <p id={describedBy} aria-live="polite" className="sr-only">
          {hovered === undefined
            ? ""
            : `${zonedDateTime(hovered.start, undefined)}, ${volumeBucketText(hovered)}`}
        </p>
      </div>
      {selection === null ? null : (
        <div className="flex h-7 items-center gap-2 text-[13px]">
          <p className="tabular-nums">
            {selectionText(volume, selection.from, selection.to, undefined)}
          </p>
          <Button type="button" size="xs" onClick={zoom}>
            Zoom to selection
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            aria-label="Clear the selection"
            onClick={() => setSelection(null)}
          >
            Clear
          </Button>
        </div>
      )}
    </div>
  )
}
