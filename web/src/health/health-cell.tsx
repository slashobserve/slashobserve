import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"
import { Skeleton } from "@/components/ui/skeleton"

import {
  healthCellText,
  kindLabels,
  repairAction,
  repairStatus,
  type HealthItem,
} from "./health-labels"
import { useListHealth, useListHealthRepair } from "./list-health"
import { KindSummary } from "./kind-summary"
import { ProblemGroups } from "./problem-groups"
import { RepairControl } from "./repair-control"

function ariaLabel(
  title: string,
  kind: string,
  more: number,
  repair: string | undefined
): string {
  const others =
    more === 0 ? "" : ` and ${more} more ${more === 1 ? "problem" : "problems"}`
  return `Health of ${title}: ${kind}${others}${repair === undefined ? "" : `, ${repair}`}`
}

function CheckLine({ item }: { item: HealthItem }) {
  if (item.skipped === "depends_on_viewer")
    return <>Skipped: reads the viewer</>
  if (item.checked_at === null) return <>Not checked yet</>
  return (
    <>
      Checked <RelativeTime iso={item.checked_at} />
      {item.checked_version === null
        ? null
        : ` on version ${item.checked_version}`}
    </>
  )
}

export function HealthCell({
  dashboardId,
  title,
}: {
  dashboardId: string
  title: string
}) {
  const { item, pending } = useListHealth(dashboardId)
  const { agentAvailable, pendingId, requestRepair } = useListHealthRepair()
  if (pending) return <Skeleton className="h-4 w-20" />
  if (item === null) return null
  const cell = healthCellText(item)
  if (cell === null) return null
  const { group, more } = cell
  const status = repairStatus(repairAction(item, agentAvailable))
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="xs"
            aria-haspopup="dialog"
            aria-label={ariaLabel(
              title,
              kindLabels[group.kind],
              more,
              status?.label
            )}
            className="-ml-2 h-auto max-w-full flex-col items-start gap-0.5 py-1 text-left font-normal whitespace-normal"
          />
        }
      >
        <span className="flex flex-wrap items-center gap-x-1.5">
          <KindSummary group={group} />
          {more === 0 ? null : (
            <span className="text-muted-foreground">+{more} more</span>
          )}
        </span>
        {status === null ? null : (
          <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
        )}
      </PopoverTrigger>
      <PopoverContent align="start" className="w-80">
        <PopoverTitle>Health</PopoverTitle>
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
        <ProblemGroups problems={item.problems} />
        <div>
          <RepairControl
            item={item}
            agentAvailable={agentAvailable}
            pending={pendingId === item.id}
            onRepair={() => requestRepair(item.id)}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}
