import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import { Popover, PopoverTrigger } from "@/components/ui/popover"

import {
  awaitsRecheck,
  dashboardProblems,
  isHealthy,
  problemCountText,
  recheckPendingLabel,
  repairAction,
  repairLabel,
  repairStatus,
  worstTone,
  type HealthItem,
  type SummaryLabel,
} from "./health-labels"
import { HealthPopoverContent, HealthyPopover } from "./health-popover"
import { RecheckPending } from "./recheck-pending"
import { RepairControl } from "./repair-control"
import { useDashboardHealth } from "./use-dashboard-health"

function HealthyButton({ item }: { item: HealthItem }) {
  return (
    <HealthyPopover
      item={item}
      align="end"
      trigger={<Button variant="ghost" size="sm" aria-haspopup="dialog" />}
    />
  )
}

export function DashboardHealthButton({
  dashboardId,
  blockedReason,
}: {
  dashboardId: string
  blockedReason: string | null
}) {
  const { item, agentAvailable, repair, pending } =
    useDashboardHealth(dashboardId)
  if (item === null) return null
  const awaiting = awaitsRecheck(item)
  const problems = dashboardProblems(item)
  const status = repairStatus(repairAction(item, agentAvailable))
  if (awaiting && problems.length === 0 && status === null) {
    return <RecheckPending item={item} variant="ghost" size="sm" align="end" />
  }
  if (isHealthy(item)) return <HealthyButton item={item} />
  if (problems.length === 0 && item.repair === null) return null
  const summary: SummaryLabel =
    awaiting && problems.length === 0
      ? { tone: "none", label: recheckPendingLabel }
      : { tone: worstTone(problems), label: problemCountText(problems.length) }
  const label: SummaryLabel =
    (status?.tone === "running" || problems.length === 0 ? status : null) ??
    summary
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="outline" size="sm" aria-haspopup="dialog" />}
      >
        <StatusLabel tone={label.tone}>{label.label}</StatusLabel>
        {awaiting && label.label !== recheckPendingLabel ? (
          <span className="text-muted-foreground">{recheckPendingLabel}</span>
        ) : null}
      </PopoverTrigger>
      <HealthPopoverContent item={item} problems={problems} align="end">
        <RepairControl
          item={item}
          agentAvailable={agentAvailable}
          pending={pending}
          onRepair={() => repair()}
          blockedReason={blockedReason}
          label={repairLabel(problems)}
        />
      </HealthPopoverContent>
    </Popover>
  )
}
