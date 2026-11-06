import { useMemo } from "react"

import { tailVolume } from "./tail-state"

export function TailVolume({
  times,
  now,
  noun,
}: {
  times: readonly number[]
  now: number
  noun: string
}) {
  const buckets = useMemo(() => tailVolume(times, now), [times, now])
  const highest = Math.max(1, ...buckets.map((bucket) => bucket.count))
  return (
    <div className="flex shrink-0 flex-col gap-1 border-b border-border px-3 py-2">
      <div
        role="img"
        aria-label={`${noun[0].toUpperCase()}${noun.slice(1)} per 10 seconds over the last 5 minutes, newest on the right`}
        className="flex h-12 items-end gap-px"
      >
        {buckets.map((bucket) => (
          <div
            key={bucket.start}
            className="min-h-px min-w-0 flex-1 bg-series-1"
            style={{
              height:
                bucket.count === 0 ? 1 : `${(bucket.count / highest) * 100}%`,
            }}
          />
        ))}
      </div>
      <p className="text-xs text-muted-foreground">
        10 s buckets (auto), newest on the right
      </p>
    </div>
  )
}
