import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import { CheckLine } from "./check-line"
import {
  dashboardProblems,
  healthyLabel,
  isHealthy,
  problemCountText,
  repairAction,
  repairLabel,
  repairStatus,
  worstTone,
  type HealthItem,
  type SummaryLabel,
} from "./health-labels"
import { ProblemGroups } from "./problem-groups"
import { RepairControl } from "./repair-control"
import { useDashboardHealth } from "./use-dashboard-health"

function HealthyButton({ item }: { item: HealthItem }) {
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="ghost" size="sm" aria-haspopup="dialog" />}
      >
        <StatusLabel tone="ok">{healthyLabel}</StatusLabel>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverTitle>Health</PopoverTitle>
        <p>No problems found</p>
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
      </PopoverContent>
    </Popover>
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
  if (isHealthy(item)) return <HealthyButton item={item} />
  const problems = dashboardProblems(item)
  if (problems.length === 0 && item.repair === null) return null
  const status = repairStatus(repairAction(item, agentAvailable))
  const label: SummaryLabel = (status?.tone === "running" ||
  problems.length === 0
    ? status
    : null) ?? {
    tone: worstTone(problems),
    label: problemCountText(problems.length),
  }
  return (
    <Popover>
      <PopoverTrigger
        render={<Button variant="outline" size="sm" aria-haspopup="dialog" />}
      >
        <StatusLabel tone={label.tone}>{label.label}</StatusLabel>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverTitle>Health</PopoverTitle>
        <p className="text-xs text-muted-foreground">
          <CheckLine item={item} />
        </p>
        <ProblemGroups problems={problems} />
        <div>
          <RepairControl
            item={item}
            agentAvailable={agentAvailable}
            pending={pending}
            onRepair={() => repair()}
            blockedReason={blockedReason}
            label={repairLabel(problems)}
          />
        </div>
      </PopoverContent>
    </Popover>
  )
}
