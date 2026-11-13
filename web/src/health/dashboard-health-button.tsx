import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import { Button } from "@/components/ui/button"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import {
  dashboardProblems,
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
      {item.state === "outdated" ? ", before the latest version" : null}
    </>
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
