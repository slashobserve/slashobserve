import { useQuery } from "@tanstack/react-query"

import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"
import type { LogLevelKind } from "@/panels/logs/logs-model"

import {
  logVolumeQuery,
  volumeCaption,
  volumeKindNames,
  volumeKinds,
  volumeLabel,
  volumeTotals,
  type LogVolumeBucket,
} from "./volume"

const kindClasses: Record<LogLevelKind, string> = {
  error: "bg-status-critical",
  warning: "bg-status-warning",
  info: "bg-series-1",
  other: "bg-muted-foreground/40",
}

function bucketTotal(bucket: LogVolumeBucket): number {
  return volumeKinds.reduce((sum, kind) => sum + bucket[kind], 0)
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
  const volume = useQuery(logVolumeQuery(uid, { query, from, to }))
  if (volume.isError) return null
  if (volume.data === undefined) {
    return <Skeleton aria-hidden="true" className="h-12 rounded-control" />
  }
  const { buckets, total } = volume.data
  const totals = volumeTotals(buckets)
  const present = volumeKinds.filter((kind) => totals[kind] > 0)
  const highest = Math.max(1, ...buckets.map(bucketTotal))
  return (
    <div className="flex h-12 flex-col gap-1">
      <div className="flex items-center justify-between gap-3 text-xs leading-4 text-muted-foreground">
        <p>{volumeCaption(total, from, to)}</p>
        <ul className="flex items-center gap-3">
          {present.map((kind) => (
            <li key={kind} className="flex items-center gap-1.5">
              <span
                aria-hidden="true"
                className={cn("size-2 rounded-full", kindClasses[kind])}
              />
              {volumeKindNames[kind]}
            </li>
          ))}
        </ul>
      </div>
      <div
        role="img"
        aria-label={volumeLabel(total, totals)}
        className="flex min-h-0 flex-1 items-end gap-px"
      >
        {buckets.map((bucket) => {
          const lines = bucketTotal(bucket)
          return (
            <div
              key={bucket.start}
              className="flex min-w-0 flex-1 flex-col-reverse"
              style={{ height: `${(lines / highest) * 100}%` }}
            >
              {volumeKinds.map((kind) =>
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
    </div>
  )
}
