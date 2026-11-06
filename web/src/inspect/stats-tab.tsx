import { useMemo } from "react"
import { RotateCwIcon } from "lucide-react"

import { Button } from "@/components/ui/button"
import { zonedDateTime } from "@/lib/time"

import { WaitingForData } from "./data-tab"
import type { InspectTarget } from "./inspect-pane"
import type { RequestTiming } from "./request-timing"
import { frameStats } from "./stats"

function timingText(timing: RequestTiming): string {
  if (timing.startedAt !== null) return "Running"
  if (timing.lastMs === null) return "Measured on the next request"
  const text = `${Math.round(timing.lastMs)} ms`
  return timing.failed ? `${text}, failed` : text
}

function Fact({ term, children }: { term: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{term}</dt>
      <dd className="min-w-0 wrap-anywhere tabular-nums">{children}</dd>
    </>
  )
}

export function StatsTab({
  target,
  timing,
  zone,
}: {
  target: InspectTarget | null
  timing: RequestTiming
  zone: string | undefined
}) {
  const frames = target?.frames
  const stats = useMemo(
    () => (frames ? frameStats(frames) : undefined),
    [frames]
  )
  if (!target || !target.data || !stats) return <WaitingForData />
  const { meta } = target.data
  return (
    <div className="flex flex-col gap-4">
      <dl className="grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 text-[13px]">
        <Fact term="Request time">{timingText(timing)}</Fact>
        <Fact term="From the cache">{meta.cached ? "Yes" : "No"}</Fact>
        <Fact term="Ran at">{zonedDateTime(meta.executed_at, zone)}</Fact>
        {meta.source_panel ? (
          <Fact term="Results of">
            <span className="font-mono text-xs">{meta.source_panel}</span>
          </Fact>
        ) : null}
        <Fact term="Frames">{stats.frames}</Fact>
        <Fact term="Series">{stats.series}</Fact>
        <Fact term="Points">{stats.points}</Fact>
        <Fact term="Rows">{stats.rows}</Fact>
        <Fact term="First time">
          {stats.minTime === null ? "None" : zonedDateTime(stats.minTime, zone)}
        </Fact>
        <Fact term="Last time">
          {stats.maxTime === null ? "None" : zonedDateTime(stats.maxTime, zone)}
        </Fact>
      </dl>
      <div>
        <Button
          variant="outline"
          size="sm"
          disabled={timing.startedAt !== null}
          onClick={target.refetch}
        >
          <RotateCwIcon aria-hidden="true" />
          Run again
        </Button>
      </div>
    </div>
  )
}
