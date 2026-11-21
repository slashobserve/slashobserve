import { useRef, useState, type KeyboardEvent, type PointerEvent } from "react"
import { useQuery } from "@tanstack/react-query"

import type { AuditWindowKey } from "@/api/keys"
import { Skeleton } from "@/components/ui/skeleton"
import { useCoarsePointer } from "@/lib/coarse-pointer"
import { formatDateTime } from "@/lib/time"
import { timeTicks } from "@/lib/time-ticks"
import { cn } from "@/lib/utils"
import { bucketAt, selectionOf, walkBucket } from "@/logs/brush"

import {
  barHeight,
  bucketRange,
  eventCountText,
  stepText,
  tickAlign,
  type BucketRange,
} from "./histogram"
import { auditHistogramQuery } from "./queries"

const maxTicks = 6

const tickShift = {
  start: "",
  middle: "-translate-x-1/2",
  end: "-translate-x-full",
}

type Bucket = { time: string; count: number }

export function AuditHistogram({
  window,
  liveCount,
  onRange,
}: {
  window: AuditWindowKey
  liveCount: number
  onRange: (range: BucketRange) => void
}) {
  const histogram = useQuery(auditHistogramQuery(window))
  if (histogram.isError) return null
  if (histogram.data === undefined) {
    return <Skeleton className="h-[76px] w-full" />
  }
  return (
    <Bars
      window={window}
      buckets={histogram.data.buckets}
      total={histogram.data.total + liveCount}
      stepMs={histogram.data.step_ms}
      onRange={onRange}
    />
  )
}

function Bars({
  window,
  buckets,
  total,
  stepMs,
  onRange,
}: {
  window: AuditWindowKey
  buckets: readonly Bucket[]
  total: number
  stepMs: number
  onRange: (range: BucketRange) => void
}) {
  const count = buckets.length
  const brushable = !useCoarsePointer()
  const times = buckets.map((bucket) => Date.parse(bucket.time))
  const highest = Math.max(1, ...buckets.map((bucket) => bucket.count))
  const ticks = timeTicks(times, maxTicks, undefined)
  const step = stepText(stepMs)
  const bars = useRef<(HTMLButtonElement | null)[]>([])
  const [hover, setHover] = useState<number | null>(null)
  const [focused, setFocused] = useState<number | null>(null)
  const [drag, setDrag] = useState<{ anchor: number; to: number } | null>(null)
  const [keyAnchor, setKeyAnchor] = useState<number | null>(null)

  const pointed = hover ?? focused
  const shown = pointed === null ? undefined : buckets[pointed]
  const preview =
    drag !== null
      ? selectionOf(drag.anchor, drag.to)
      : keyAnchor !== null && focused !== null
        ? selectionOf(keyAnchor, focused)
        : null

  function apply(a: number, b: number) {
    const range = bucketRange(times, stepMs, window, a, b)
    if (range !== null) onRange(range)
  }

  function indexUnder(event: PointerEvent<HTMLDivElement>): number {
    const rect = event.currentTarget.getBoundingClientRect()
    return bucketAt(event.clientX - rect.left, rect.width, count)
  }

  function onPointerDown(event: PointerEvent<HTMLDivElement>) {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    const index = indexUnder(event)
    setDrag({ anchor: index, to: index })
  }

  function onPointerMove(event: PointerEvent<HTMLDivElement>) {
    const index = indexUnder(event)
    setHover(index)
    if (!brushable) return
    setDrag((current) => (current ? { ...current, to: index } : current))
  }

  function onPointerUp(event: PointerEvent<HTMLDivElement>) {
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    if (drag === null) return
    setDrag(null)
    apply(drag.anchor, brushable ? indexUnder(event) : drag.anchor)
  }

  function onPointerCancel() {
    setDrag(null)
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    if (event.key === "Escape") {
      setKeyAnchor(null)
      return
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault()
      apply(keyAnchor ?? index, index)
      setKeyAnchor(null)
      return
    }
    const next = walkBucket(event.key, index, count)
    if (next === null) return
    event.preventDefault()
    setKeyAnchor(event.shiftKey ? (keyAnchor ?? index) : null)
    bars.current[next]?.focus()
  }

  return (
    <div className="flex flex-col gap-1 rounded-card border px-4 py-3">
      <p className="text-xs text-muted-foreground tabular-nums">
        {eventCountText(total)}, per {step}
      </p>
      <div className="relative">
        <div
          role="group"
          aria-label={`Audit events per ${step} from ${formatDateTime(window.since)} to ${formatDateTime(window.until)}, ${total} in all, oldest on the left. Click a bar or drag across bars to set the time range.`}
          className={cn(
            "flex h-12 items-end gap-px select-none",
            brushable && "cursor-col-resize touch-none"
          )}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerCancel}
          onPointerLeave={() => setHover(null)}
          onBlur={(event) => {
            if (event.currentTarget.contains(event.relatedTarget)) return
            setFocused(null)
            setKeyAnchor(null)
          }}
        >
          {buckets.map((bucket, index) => (
            <button
              key={bucket.time}
              ref={(element) => {
                bars.current[index] = element
              }}
              type="button"
              tabIndex={index === (focused ?? count - 1) ? 0 : -1}
              aria-label={`${formatDateTime(bucket.time)}, ${eventCountText(bucket.count)}`}
              className="flex h-full min-w-0 flex-1 items-end rounded-chip outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onFocus={() => setFocused(index)}
              onKeyDown={(event) => onKeyDown(event, index)}
            >
              <span
                aria-hidden="true"
                className="block min-h-px w-full bg-series-1"
                style={{ height: barHeight(bucket.count, highest) }}
              />
            </button>
          ))}
        </div>
        {preview === null ? null : (
          <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-0 border-x border-ring bg-selection/40"
            style={{
              left: `${(preview.from / count) * 100}%`,
              width: `${((preview.to - preview.from + 1) / count) * 100}%`,
            }}
          />
        )}
        {shown === undefined || pointed === null ? null : (
          <Tooltip
            ratio={(pointed + 0.5) / count}
            time={formatDateTime(shown.time)}
            count={eventCountText(shown.count)}
          />
        )}
      </div>
      <div
        aria-hidden="true"
        className="relative h-4 text-xs text-muted-foreground"
      >
        {ticks.map((tick) => (
          <span
            key={tick.index}
            className={cn(
              "absolute whitespace-nowrap",
              tickShift[tickAlign(tick.index, count)]
            )}
            style={{ left: `${(tick.index / count) * 100}%` }}
          >
            {tick.label}
          </span>
        ))}
      </div>
    </div>
  )
}

function Tooltip({
  ratio,
  time,
  count,
}: {
  ratio: number
  time: string
  count: string
}) {
  return (
    <div
      role="tooltip"
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
      <p className="text-muted-foreground tabular-nums">{time}</p>
      <p className="tabular-nums">{count}</p>
    </div>
  )
}
