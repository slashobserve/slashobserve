import { useQuery } from "@tanstack/react-query"

import type { AuditWindowKey } from "@/api/keys"
import { Skeleton } from "@/components/ui/skeleton"
import { formatDateTime } from "@/lib/time"
import { timeTicks } from "@/lib/time-ticks"

import { barHeight, stepText } from "./histogram"
import { auditHistogramQuery } from "./queries"

const maxTicks = 6

export function AuditHistogram({ window }: { window: AuditWindowKey }) {
  const histogram = useQuery(auditHistogramQuery(window))
  if (histogram.isError) return null
  if (histogram.data === undefined) {
    return <Skeleton className="h-[76px] w-full" />
  }
  const { buckets, total, step_ms: stepMs } = histogram.data
  const times = buckets.map((bucket) => Date.parse(bucket.time))
  const highest = Math.max(1, ...buckets.map((bucket) => bucket.count))
  const ticks = timeTicks(times, maxTicks, undefined)
  const step = stepText(stepMs)
  return (
    <div className="flex flex-col gap-1 rounded-card border px-4 py-3">
      <p className="text-xs text-muted-foreground tabular-nums">
        {total.toLocaleString("en-US")} {total === 1 ? "event" : "events"}, per{" "}
        {step}
      </p>
      <div
        role="img"
        aria-label={`Audit events per ${step} from ${formatDateTime(window.since)} to ${formatDateTime(window.until)}, ${total} in all, oldest on the left`}
        className="flex h-12 items-end gap-px"
      >
        {buckets.map((bucket) => (
          <div
            key={bucket.time}
            title={`${formatDateTime(bucket.time)}: ${bucket.count}`}
            className="min-h-px min-w-0 flex-1 bg-series-1"
            style={{ height: barHeight(bucket.count, highest) }}
          />
        ))}
      </div>
      <div
        aria-hidden="true"
        className="relative h-4 overflow-hidden text-xs text-muted-foreground"
      >
        {ticks.map((tick) => (
          <span
            key={tick.index}
            className="absolute whitespace-nowrap"
            style={{ left: `${(tick.index / buckets.length) * 100}%` }}
          >
            {tick.label}
          </span>
        ))}
      </div>
    </div>
  )
}
