import { RelativeTime } from "@/components/relative-time"
import { StatusLabel } from "@/components/status-label"
import {
  Popover,
  PopoverContent,
  PopoverTitle,
  PopoverTrigger,
} from "@/components/ui/popover"

import {
  kindLabels,
  kindTones,
  problemText,
  sortProblems,
  worstTone,
  type HealthItem,
  type HealthProblem,
} from "./health-labels"
import { RepairControl } from "./repair-control"

type Props = {
  title: string
  problems: readonly HealthProblem[]
  item: HealthItem
  agentAvailable: boolean
  pending: boolean
  onRepair: () => void
  blockedReason: string | null
}

export function PanelHealthBadge({
  title,
  problems,
  item,
  agentAvailable,
  pending,
  onRepair,
  blockedReason,
}: Props) {
  const [first] = sortProblems(problems)
  if (first === undefined) return null
  const firstText = problemText(first)
  return (
    <Popover>
      <PopoverTrigger
        render={
          <button
            type="button"
            aria-haspopup="dialog"
            aria-label={`${title}: ${kindLabels[first.kind]}.${firstText === null ? "" : ` ${firstText}`}`}
            className="shrink-0 rounded-control px-1 text-xs whitespace-nowrap text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
            data-health=""
          />
        }
      >
        <StatusLabel tone={worstTone(problems)}>
          {kindLabels[first.kind]}
        </StatusLabel>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80">
        <PopoverTitle>{title}</PopoverTitle>
        <ul className="flex flex-col gap-1.5">
          {sortProblems(problems).map((problem, index) => {
            const text = problemText(problem)
            return (
              <li key={index} className="flex flex-col items-start gap-0.5">
                <StatusLabel tone={kindTones[problem.kind]}>
                  {kindLabels[problem.kind]}
                </StatusLabel>
                {text === null ? null : (
                  <span className="text-xs wrap-anywhere text-muted-foreground">
                    {text}
                  </span>
                )}
              </li>
            )
          })}
        </ul>
        <p className="text-xs text-muted-foreground">
          Checked <RelativeTime iso={item.checked_at} />
        </p>
        {item.can_edit ? (
          <>
            <p className="text-xs text-muted-foreground">
              Repair asks the agent to fix every failing panel and variable on
              this dashboard in one revision.
            </p>
            <div>
              <RepairControl
                item={item}
                agentAvailable={agentAvailable}
                pending={pending}
                onRepair={onRepair}
                blockedReason={blockedReason}
              />
            </div>
          </>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}
