import { ClockIcon } from "lucide-react"
import { toast } from "sonner"

import { TimeRangeControl } from "@/dashboards/time-range-picker"
import { describeRange, quickRanges, type QuickRange } from "@/lib/time"

import {
  liveHour,
  liveHourLabel,
  pageRangeFor,
  type PageRange,
} from "./blocks/block-data"
import { blockWriteProblem } from "./errors"
import { useUpdateObservation, type Observation } from "./queries"

const pageQuickRanges: QuickRange[] = quickRanges.map((range) =>
  range.from === liveHour.from && range.to === liveHour.to
    ? { ...range, label: liveHourLabel }
    : range
)

export function pageRangeValue(page: PageRange): { from: string; to: string } {
  return page.from !== null && page.to !== null
    ? { from: page.from, to: page.to }
    : liveHour
}

export function PageRangeControl({
  observation,
}: {
  observation: Observation
}) {
  const update = useUpdateObservation(observation.id)
  const value = pageRangeValue(observation)

  if (!observation.allowed_actions.includes("update")) {
    return (
      <span className="inline-flex h-[30px] items-center gap-1.5 px-1 text-sm text-muted-foreground">
        <ClockIcon aria-hidden="true" className="size-4" />
        {describeRange(value.from, value.to, pageQuickRanges)}
      </span>
    )
  }

  return (
    <TimeRangeControl
      value={value}
      quickRanges={pageQuickRanges}
      onChange={(next) =>
        update.mutate(pageRangeFor(next, Date.now()), {
          onError: (error) => toast.error(blockWriteProblem(error).text),
        })
      }
    />
  )
}
