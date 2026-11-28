import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Popover, PopoverTrigger } from "@/components/ui/popover"
import { Skeleton } from "@/components/ui/skeleton"

import {
  awaitsRecheck,
  currentProblems,
  healthCellText,
  healthyLabel,
  isHealthy,
  kindLabels,
  moreProblemsText,
  recheckPendingLabel,
  repairAction,
  repairStatus,
  type HealthItem,
} from "./health-labels"
import { useListHealth, useListHealthRepair } from "./list-health"
import { KindSummary } from "./kind-summary"
import { HealthPopoverContent, HealthyPopover } from "./health-popover"
import { RecheckPending } from "./recheck-pending"
import { RepairControl } from "./repair-control"

function ariaLabel(title: string, parts: readonly (string | undefined)[]) {
  return `Health of ${title}: ${parts.filter((part) => part !== undefined).join(", ")}`
}

function HealthyCell({ item, title }: { item: HealthItem; title: string }) {
  return (
    <HealthyPopover
      item={item}
      align="start"
      trigger={
        <Button
          variant="ghost"
          size="xs"
          aria-haspopup="dialog"
          aria-label={`Health of ${title}: ${healthyLabel}`}
          className="-ml-2 h-auto max-w-full py-1 text-left font-normal whitespace-normal"
        />
      }
    />
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
  const awaiting = awaitsRecheck(item)
  const cell = healthCellText(item)
  const status = repairStatus(repairAction(item, agentAvailable))
  if (cell === null && !awaiting) {
    return isHealthy(item) ? <HealthyCell item={item} title={title} /> : null
  }
  if (cell === null && status === null) {
    return (
      <RecheckPending
        item={item}
        title={title}
        variant="ghost"
        size="xs"
        align="start"
        className="-ml-2 h-auto max-w-full py-1 text-left font-normal whitespace-normal"
      />
    )
  }
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            variant="ghost"
            size="xs"
            aria-haspopup="dialog"
            aria-label={ariaLabel(title, [
              cell === null
                ? undefined
                : `${kindLabels[cell.group.kind]}${cell.more === 0 ? "" : ` and ${moreProblemsText(cell.more)}`}`,
              awaiting ? recheckPendingLabel : undefined,
              status?.label,
            ])}
            className="-ml-2 h-auto max-w-full flex-col items-start gap-0.5 py-1 text-left font-normal whitespace-normal"
          />
        }
      >
        {cell === null ? null : (
          <span className="flex flex-wrap items-center gap-x-1.5">
            <KindSummary group={cell.group} />
            {cell.more === 0 ? null : (
              <span className="text-muted-foreground">
                +{moreProblemsText(cell.more)}
              </span>
            )}
          </span>
        )}
        {awaiting ? (
          <StatusLabel tone="none">{recheckPendingLabel}</StatusLabel>
        ) : null}
        {status === null ? null : (
          <StatusLabel tone={status.tone}>{status.label}</StatusLabel>
        )}
      </PopoverTrigger>
      <HealthPopoverContent
        item={item}
        problems={currentProblems(item)}
        align="start"
      >
        <RepairControl
          item={item}
          agentAvailable={agentAvailable}
          pending={pendingId === item.id}
          onRepair={() => requestRepair(item.id)}
        />
      </HealthPopoverContent>
    </Popover>
  )
}
